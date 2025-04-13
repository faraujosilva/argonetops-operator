/*
Copyright 2025.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package controller

import (
	"context"
	"fmt"
	"strings"
	"time"

	gomiko "github.com/Ali-aqrabawi/gomiko/pkg"

	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"

	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkv1alpha1 "github.com/faraujosilva/argonetops-operator/api/v1alpha1"
)

// DeviceConfigReconciler reconciles a DeviceConfig object
type DeviceConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

// +kubebuilder:rbac:groups=network.argonetops.io,resources=deviceconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.argonetops.io,resources=deviceconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=network.argonetops.io,resources=deviceconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the DeviceConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile

func (r *DeviceConfigReconciler) cleanupDevice(ctx context.Context, devConfig *networkv1alpha1.DeviceConfig) error {
	log := log.FromContext(ctx)
	log.Info("Iniciando rollback (cleanup) do DeviceConfig", "device", devConfig.Name)

	username, password, err := getCredentials(r.Client, ctx, devConfig.Namespace)
	if err != nil {
		log.Error(err, "Falha ao obter credenciais durante rollback do DeviceConfig")
		return err
	}

	device, err := gomiko.NewDevice(
		devConfig.Spec.IP,
		username,
		password,
		devConfig.Spec.DeviceType,
		devConfig.Spec.Port,
	)
	if err != nil {
		log.Error(err, "Erro ao criar dispositivo durante rollback")
		return err
	}

	if err := device.Connect(); err != nil {
		log.Error(err, "Erro ao conectar ao dispositivo durante rollback")
		return err
	}
	defer device.Disconnect()

	rollbackCommands := []string{
		"default hostname",
	}

	if _, err := device.SendConfigSet(rollbackCommands); err != nil {
		log.Error(err, "Erro ao executar rollback no dispositivo")
		return err
	}

	log.Info("Rollback (cleanup) concluído com sucesso", "device", devConfig.Name)

	return nil
}

func (r *DeviceConfigReconciler) handleError(ctx context.Context, devConfig *networkv1alpha1.DeviceConfig, err error, message string) {
	log := log.FromContext(ctx)
	log.Error(err, message, "deviceIP", devConfig.Spec.IP)

	devConfig.Status.State = ErrorState
	devConfig.Status.Message = message + ": " + err.Error()
	if updateErr := r.Status().Update(ctx, devConfig); updateErr != nil {
		log.Error(updateErr, "Erro ao atualizar status do objeto após erro")
	}
}

func getCredentials(client client.Client, ctx context.Context, namespace string) (string, string, error) {
	var configMap corev1.ConfigMap
	if err := client.Get(ctx, types.NamespacedName{Name: "device-credentials", Namespace: namespace}, &configMap); err != nil {
		return "", "", err
	}

	username, ok := configMap.Data["username"]
	if !ok {
		return "", "", fmt.Errorf("username key %s not found in ConfigMap %s", "username", "device-credentials")
	}

	password, ok := configMap.Data["password"]
	if !ok {
		return "", "", fmt.Errorf("password key %s not found in ConfigMap %s", "password", "device-credentials")
	}

	return username, password, nil
}

func (r *DeviceConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	const devConfigFinalizer = "deviceconfig.network.argonetops.io/finalizer"

	var devConfig networkv1alpha1.DeviceConfig
	if err := r.Get(ctx, req.NamespacedName, &devConfig); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	if devConfig.ObjectMeta.DeletionTimestamp.IsZero() {
		// Se o objeto está sendo excluído, apenas execute cleanup
		if !controllerutil.ContainsFinalizer(&devConfig, devConfigFinalizer) {
			controllerutil.AddFinalizer(&devConfig, devConfigFinalizer)
			if err := r.Update(ctx, &devConfig); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// Objeto marcado para deleção
		if controllerutil.ContainsFinalizer(&devConfig, devConfigFinalizer) {
			// Remova o finalizer do devConfig
			if err := r.cleanupDevice(ctx, &devConfig); err != nil {
				r.handleError(ctx, &devConfig, err, "Erro ao fazer rollback (cleanup) do DeviceConfig")
				return ctrl.Result{RequeueAfter: 10 * time.Second}, nil
			}
			controllerutil.RemoveFinalizer(&devConfig, devConfigFinalizer)
			if err := r.Update(ctx, &devConfig); err != nil {
				return ctrl.Result{}, r.Update(ctx, &devConfig)
			}

			return ctrl.Result{}, nil
		}
	}

	log := log.FromContext(ctx)

	// Obtenha as credenciais do ConfigMap
	// TODO: Pegar credentcial baseado no metadata
	username, password, err := getCredentials(r.Client, ctx, req.Namespace)
	if err != nil {
		r.handleError(ctx, &devConfig, err, "Erro ao buscar credenciais")
		return ctrl.Result{}, err
	}

	log.Info("Criando nova conexão", "Hostname", devConfig.Spec.Hostname)
	device, err := gomiko.NewDevice(
		devConfig.Spec.IP,
		username,
		password,
		devConfig.Spec.DeviceType,
		devConfig.Spec.Port,
	)

	if device == nil {
		err := fmt.Errorf("dispositivo não foi inicializado corretamente")
		r.handleError(ctx, &devConfig, err, "Erro ao criar dispositivo")
		return ctrl.Result{}, err
	}
	log.Info("Dispositivo criado com sucesso", "deviceIP", devConfig.Spec.IP)
	if err := device.Connect(); err != nil {
		log.Error(err, "Erro ao conectar ao dispositivo", "deviceIP", devConfig.Spec.IP)
		r.handleError(ctx, &devConfig, err, "Erro 3 ao conectar ao dispositivo")
		return ctrl.Result{}, err
	}

	defer device.Disconnect()
	var currentHostname string

	log.Info("Coletando hostname atual do dispositivo", "deviceIP", devConfig.Spec.IP)
	cmdHostname, err := device.SendCommand("show run")
	// exit after run command
	if err != nil {
		r.handleError(ctx, &devConfig, err, "Erro ao coletar hostname atual")
		return ctrl.Result{}, err
	}
	for _, line := range strings.Split(cmdHostname, "\n") {
		if strings.Contains(line, "hostname") {
			log.Info("Linha do hostname", "linha", line)
			currentHostname = strings.Split(line, " ")[1]
			currentHostname = strings.TrimSpace(currentHostname)
			break
		}
	}
	log.Info("Hostname atual", "hostname", currentHostname)

	if currentHostname != devConfig.Spec.Hostname {
		log.Info("Hostname diferente do desejado, alterando...", "hostname", devConfig.Spec.Hostname)
		confSet := []string{
			fmt.Sprintf("hostname %s", devConfig.Spec.Hostname),
		}
		_, err := device.SendConfigSet(confSet)
		if err != nil {
			r.handleError(ctx, &devConfig, err, "Erro ao enviar configuração")
			return ctrl.Result{}, err
		}

		log.Info("Hostname alterado com sucesso", "hostname", devConfig.Spec.Hostname)
		devConfig.Status.State = SuccessState
		devConfig.Status.Message = "Hostname alterado com sucesso"
		if err := r.Status().Update(ctx, &devConfig); err != nil {
			log.Error(err, "Erro ao atualizar status do objeto")
			return ctrl.Result{}, err
		}
	} else {
		log.Info("Hostname já está correto", "hostname", currentHostname)
		devConfig.Status.State = SuccessState
		devConfig.Status.Message = "Hostname já está correto"
		if err := r.Status().Update(ctx, &devConfig); err != nil {
			log.Error(err, "Erro ao atualizar status do objeto")
			return ctrl.Result{}, err
		}
	}

	log.Info("Conexão com o dispositivo fechada", "deviceIP", devConfig.Spec.IP)

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil

}

// SetupWithManager sets up the controller with the Manager.
func (r *DeviceConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv1alpha1.DeviceConfig{}).
		Named("deviceconfig").
		Complete(r)
}
