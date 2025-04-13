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
	"strconv"
	"strings"
	"time"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	gomiko "github.com/Ali-aqrabawi/gomiko/pkg"
	networkv1alpha1 "github.com/faraujosilva/argonetops-operator/api/v1alpha1"
	"github.com/faraujosilva/argonetops-operator/internal/parsers"
)

// InterfaceConfigReconciler reconciles a InterfaceConfig object
type InterfaceConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

const (
	ErrorState   = "Error"
	SuccessState = "Success"
)

// +kubebuilder:rbac:groups=network.argonetops.io,resources=interfaceconfigs,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=network.argonetops.io,resources=interfaceconfigs/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=network.argonetops.io,resources=interfaceconfigs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the InterfaceConfig object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.20.4/pkg/reconcile

// dentro do método rollbackConfig
func (r *InterfaceConfigReconciler) rollbackConfig(ctx context.Context, iface networkv1alpha1.InterfaceConfig) error {
	log := log.FromContext(ctx)
	log.Info("Aplicando rollback", "name", iface.Name)

	username, password, err := getCredentials(r.Client, ctx, iface.Namespace)
	if err != nil {
		return err
	}

	devicePortInt, _ := strconv.Atoi(iface.Labels["devicePort"])
	devicePort := uint8(devicePortInt)

	device, err := gomiko.NewDevice(
		iface.Labels["deviceIP"],
		username,
		password,
		iface.Labels["deviceType"],
		devicePort,
	)
	if err != nil {
		return err
	}

	if err := device.Connect(); err != nil {
		return err
	}
	defer device.Disconnect()

	rollbackCommands := []string{
		"interface " + iface.Spec.InterfaceName,
		"no description",
		"no ip address",
		"shutdown",
	}
	if _, err = device.SendConfigSet(rollbackCommands); err != nil {
		return err
	}

	return nil
}

func (r *InterfaceConfigReconciler) handleError(ctx context.Context, devHostname string, iface *networkv1alpha1.InterfaceConfig, err error, message string) {
	log := log.FromContext(ctx)
	log.Error(err, message, "Hostname", devHostname, "interfaceName", iface.Spec.InterfaceName)

	iface.Status.State = ErrorState
	iface.Status.Message = message + ": " + err.Error()
	if updateErr := r.Status().Update(ctx, iface); updateErr != nil {
		log.Error(updateErr, "Erro ao atualizar status do objeto após erro")
	}
}

func (r *InterfaceConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	const interfaceFinalizer = "network.argonetops.io/finalizer"

	var iface networkv1alpha1.InterfaceConfig
	log := log.FromContext(ctx)

	if err := r.Get(ctx, req.NamespacedName, &iface); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	// Verifica se o DeviceConfig associado existe
	var devConfig networkv1alpha1.DeviceConfig
	deviceConfigKey := types.NamespacedName{Name: iface.Spec.DeviceName, Namespace: req.Namespace}
	if err := r.Get(ctx, deviceConfigKey, &devConfig); err != nil {
		if client.IgnoreNotFound(err) == nil {
			return ctrl.Result{RequeueAfter: time.Second * 10}, nil
		}
	} // Verifica se o DeviceConfig tem o mesmo nome do InterfaceConfig

	// Adiciona as infos do device no metadata
	updatedLabels := map[string]string{
		"deviceName": devConfig.Spec.Hostname,
		"deviceIP":   devConfig.Spec.IP,
		"deviceType": devConfig.Spec.DeviceType,
		"devicePort": fmt.Sprintf("%d", devConfig.Spec.Port),
	}
	needsUpdate := false
	for key, value := range updatedLabels {
		if current, exists := iface.ObjectMeta.Labels[key]; !exists || current != value {
			needsUpdate = true
			break
		}
	}
	if needsUpdate && iface.ObjectMeta.DeletionTimestamp.IsZero() {
		iface.ObjectMeta.Labels = updatedLabels
		if err := r.Update(ctx, &iface); err != nil {
			log.Error(err, "Erro ao atualizar labels do InterfaceConfig")
			return ctrl.Result{}, err
		}
		return ctrl.Result{Requeue: true}, nil // requeue após atualização
	}

	devicePortStr := iface.ObjectMeta.Labels["devicePort"]
	log.Info("devicePort", "devicePort", devicePortStr)

	devicePortInt, err := strconv.Atoi(devicePortStr)
	if err != nil {
		log.Error(err, "Erro ao converter devicePort para inteiro", "devicePort", devicePortStr)
		return ctrl.Result{}, err
	}
	devicePort := uint8(devicePortInt)
	if iface.ObjectMeta.DeletionTimestamp.IsZero() {
		// Adiciona finalizer se ainda não tem
		if !controllerutil.ContainsFinalizer(&iface, interfaceFinalizer) {
			controllerutil.AddFinalizer(&iface, interfaceFinalizer)
			if err := r.Update(ctx, &iface); err != nil {
				return ctrl.Result{}, err
			}
		}
	} else {
		// Objeto marcado para deleção
		if controllerutil.ContainsFinalizer(&iface, interfaceFinalizer) {
			// Aqui entra a lógica de rollback
			// TODO: Pegar credentcial baseado no metadata
			username, password, err := getCredentials(r.Client, ctx, req.Namespace)
			if err != nil {
				r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao obter credenciais do ConfigMap")
				return ctrl.Result{}, err
			}

			log.Info("Criando nova conexão", "Hostname", iface.Spec.DeviceName)
			// reconvert metadata string devicePrort to uint8
			log.Info("devicePort", "devicePort", devicePort)

			device, err := gomiko.NewDevice(
				iface.ObjectMeta.Labels["deviceIP"],
				username,
				password,
				iface.ObjectMeta.Labels["deviceType"],
				devicePort,
			)
			if err != nil {
				r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao criar dispositivo")
				return ctrl.Result{}, err
			}

			if device == nil {
				err := fmt.Errorf("dispositivo não foi inicializado corretamente")
				r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao criar dispositivo")
				return ctrl.Result{}, err
			}
			log.Info("Dispositivo criado com sucesso", "deviceIP", iface.ObjectMeta.Labels["deviceIP"])
			if err := device.Connect(); err != nil {
				r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao conectar ao dispositivo")
				return ctrl.Result{}, err
			}
			defer device.Disconnect()
			err = r.rollbackConfig(ctx, iface)
			if err != nil {
				log.Error(err, "Erro ao aplicar rollback")
				return ctrl.Result{RequeueAfter: time.Second * 10}, nil
			}

			// Remove o finalizer após a limpeza
			log.Info("Removendo finalizer do InterfaceConfig")
			controllerutil.RemoveFinalizer(&iface, interfaceFinalizer)
			if err := r.Update(ctx, &iface); err != nil {
				log.Error(err, "Erro ao remover finalizer do InterfaceConfig")
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	// TODO: Pegar credentcial baseado no metadata
	username, password, err := getCredentials(r.Client, ctx, req.Namespace)
	if err != nil {
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao obter credenciais do ConfigMap")
		return ctrl.Result{}, err
	}

	log.Info("Criando nova conexão", "Hostname", iface.Spec.DeviceName)
	device, err := gomiko.NewDevice(
		iface.ObjectMeta.Labels["deviceIP"],
		username,
		password,
		iface.ObjectMeta.Labels["deviceType"],
		devicePort,
	)
	if err != nil {
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao criar dispositivo")
		return ctrl.Result{}, err
	}

	if device == nil {
		err := fmt.Errorf("dispositivo não foi inicializado corretamente")
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao criar dispositivo")
		return ctrl.Result{}, err
	}
	log.Info("Dispositivo criado com sucesso", "deviceIP", iface.ObjectMeta.Labels["deviceIP"])
	if err := device.Connect(); err != nil {
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao conectar ao dispositivo")
		return ctrl.Result{}, err
	}
	defer device.Disconnect()

	output, err := device.SendCommand("show running-config interface " + iface.Spec.InterfaceName)
	if err != nil {
		if strings.Contains(err.Error(), "timeout while reading, read pattern not found pattern") {
			device.Disconnect()
			log.Error(err, "Timeout ao obter configuração da interface, tentando novamente")
			return ctrl.Result{RequeueAfter: time.Second * 2}, nil
		}
		return ctrl.Result{}, err
	}

	dt := parsers.InterfaceParserFactory{
		DeviceType: devConfig.Spec.DeviceType,
	}

	parser, err := dt.GetParser()
	if err != nil {
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao obter parser para o dispositivo")
		return ctrl.Result{}, err
	}

	// Parseia a configuração atual da interface
	parsedOutput, err := parser.ParseConfig(output)
	if err != nil {
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao parsear configuração da interface")
		return ctrl.Result{}, err
	}

	intstatuscmd, err := device.SendCommand("show interface " + iface.Spec.InterfaceName + " | in line")
	if err != nil {
		if strings.Contains(err.Error(), "timeout while reading, read pattern not found pattern") {
			device.Disconnect()
			log.Error(err, "Timeout ao obter configuração da interface, tentando novamente")
			return ctrl.Result{RequeueAfter: time.Second * 2}, nil
		}
		log.Error(err, "Erro ao obter status da interface, considerando UP")
	}
	log.Info("Status da interface: ", "intstatuscmd", intstatuscmd)
	// Parseia o status da interface
	intstatus, err := parser.ParseStatus(intstatuscmd)
	if err != nil {
		r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao parsear status da interface")
		return ctrl.Result{}, err
	}
	parsedOutput.State = intstatus

	// Verifica se a interface está em shutdown
	var desiredState string
	if iface.Spec.Shutdown {
		desiredState = "DOWN"
	} else {
		desiredState = "UP"
	}

	desiredConfig := parsers.InterfaceFormatted{
		Name:  iface.Spec.InterfaceName,
		Descr: iface.Spec.Description,
		IP:    iface.Spec.IPAddress,
		Mask:  iface.Spec.SubnetMask,
		State: desiredState,
	}
	log.Info("Configuração desejada: ", "desiredConfig", desiredConfig)
	log.Info("Configuração atual: ", "parsedOutput", parsedOutput)
	// Compara a config desejada com o output formatado
	if parsedOutput.Name != desiredConfig.Name ||
		parsedOutput.Descr != desiredConfig.Descr ||
		parsedOutput.IP != desiredConfig.IP ||
		parsedOutput.Mask != desiredConfig.Mask ||
		parsedOutput.State != desiredConfig.State {
		log.Info("Configuração não corresponde, aplicando alterações")
		// Se a configuração não corresponder, aplica as alterações
		configCommands := []string{
			"interface " + iface.Spec.InterfaceName,
			"description " + iface.Spec.Description,
			"ip address " + iface.Spec.IPAddress + " " + iface.Spec.SubnetMask,
			"no shutdown",
		}
		cmds, err := device.SendConfigSet(configCommands)
		if err != nil {
			if strings.Contains(err.Error(), "timeout while reading, read pattern not found pattern") {
				device.Disconnect()
				log.Error(err, "Timeout ao obter configuração da interface, tentando novamente")
				return ctrl.Result{RequeueAfter: time.Second * 2}, nil
			}
			r.handleError(ctx, iface.Spec.DeviceName, &iface, err, "Erro ao aplicar configuração")
			return ctrl.Result{}, err
		}
		log.Info("Configuração aplicada com sucesso", "commands", cmds)
	} else {
		log.Info("Configuração já está atualizada")
		iface.Status.State = SuccessState
		iface.Status.Message = "Configuração aplicada com sucesso"
		if err := r.Status().Update(ctx, &iface); err != nil {
			return ctrl.Result{}, err
		}
	}

	return ctrl.Result{RequeueAfter: time.Second * 30}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *InterfaceConfigReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&networkv1alpha1.InterfaceConfig{}).
		Named("interfaceconfig").
		Complete(r)
}
