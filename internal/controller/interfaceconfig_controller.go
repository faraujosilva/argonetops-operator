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
	"time"

	gomiko "github.com/Ali-aqrabawi/gomiko/pkg"
	"k8s.io/apimachinery/pkg/runtime"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	networkv1alpha1 "github.com/seunome/argonetops-operator/api/v1alpha1"
	"github.com/seunome/argonetops-operator/internal/parsers"
)

// InterfaceConfigReconciler reconciles a InterfaceConfig object
type InterfaceConfigReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

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

func (r *InterfaceConfigReconciler) rollbackConfig(iface networkv1alpha1.InterfaceConfig) error {
	// Aqui você pode implementar a lógica de rollback
	// Por exemplo, restaurar a configuração anterior ou remover a interface

	// Exemplo: apenas logando a ação
	log := log.FromContext(context.Background())
	log.Info("Aplicando rollback", "name", iface.Name)

	device, err := gomiko.NewDevice(
		iface.Spec.DeviceIP,
		iface.Spec.Username,
		iface.Spec.Password,
		iface.Spec.DeviceType,
		iface.Spec.DevicePort,
	)
	if err != nil {
		log.Error(err, "Erro 1 ao conectar ao dispositivo para rollback")
		iface.Status.State = "Error"
		iface.Status.Message = err.Error()
		if err := r.Update(context.Background(), &iface); err != nil {
			log.Error(err, "Erro ao atualizar status após erro de rollback")
		}
	}

	// Connect to device
	if err := device.Connect(); err != nil {
		log.Error(err, "Erro 2 ao conectar ao dispositivo para rollback")
		iface.Status.State = "Error"
		iface.Status.Message = err.Error()
		if err := r.Status().Update(context.Background(), &iface); err != nil {
			log.Error(err, "Erro ao atualizar status após erro de rollback")
		}
	}
	defer device.Disconnect()

	rollbackCommands := []string{
		"interface " + iface.Spec.InterfaceName,
		"no description",
		"no ip address",
		"shutdown",
	}
	_, err = device.SendConfigSet(rollbackCommands)
	if err != nil {
		log.Error(err, "Erro ao aplicar rollback")
		iface.Status.State = "Error"
		iface.Status.Message = err.Error()
		if err := r.Status().Update(context.Background(), &iface); err != nil {
			log.Error(err, "Erro ao atualizar status após erro de rollback")
		}
	}

	// Atualiza o status do objeto após o rollback
	iface.Status.State = "RolledBack"
	iface.Status.Message = "Rollback aplicado com sucesso"
	if err := r.Status().Update(context.Background(), &iface); err != nil {
		log.Error(err, "Erro ao atualizar status após rollback")
	}
	// Aqui você pode adicionar lógica adicional, como restaurar a configuração anterior
	// ou remover a interface, dependendo do que você deseja fazer.
	// Exemplo: apenas logando a ação
	log.Info("Rollback aplicado com sucesso", "name", iface.Name)

	return nil
}
func (r *InterfaceConfigReconciler) handleError(ctx context.Context, iface *networkv1alpha1.InterfaceConfig, err error, message string) {
	log := log.FromContext(ctx)
	log.Error(err, message, "deviceIP", iface.Spec.DeviceIP, "interfaceName", iface.Spec.InterfaceName)

	iface.Status.State = "Error"
	iface.Status.Message = message + ": " + err.Error()
	if updateErr := r.Status().Update(ctx, iface); updateErr != nil {
		log.Error(updateErr, "Erro ao atualizar status do objeto após erro")
	}
}

func (r *InterfaceConfigReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	const interfaceFinalizer = "network.argonetops.io/finalizer"

	var iface networkv1alpha1.InterfaceConfig
	if err := r.Get(ctx, req.NamespacedName, &iface); err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
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
			err := r.rollbackConfig(iface)
			if err != nil {
				return ctrl.Result{}, err
			}

			// Remove o finalizer após a limpeza
			controllerutil.RemoveFinalizer(&iface, interfaceFinalizer)
			if err := r.Update(ctx, &iface); err != nil {
				return ctrl.Result{}, err
			}
		}
		return ctrl.Result{}, nil
	}

	log := log.FromContext(ctx)

	// Conecta ao dispositivo via SSH usando gomiko
	device, err := gomiko.NewDevice(
		iface.Spec.DeviceIP,
		iface.Spec.Username,
		iface.Spec.Password,
		iface.Spec.DeviceType,
		iface.Spec.DevicePort,
	)
	if err != nil {
		r.handleError(ctx, &iface, err, "Erro ao criar dispositivo")
		return ctrl.Result{}, err
	}

	if device == nil {
		err := fmt.Errorf("dispositivo não foi inicializado corretamente")
		r.handleError(ctx, &iface, err, "Erro ao criar dispositivo")
		return ctrl.Result{}, err
	}
	//Connect to device
	if err := device.Connect(); err != nil {
		r.handleError(ctx, &iface, err, "Erro 3 ao conectar ao dispositivo")
		return ctrl.Result{}, err
	}

	defer device.Disconnect()
	// Coleta estado atual da interface
	output, err := device.SendCommand("show running-config interface " + iface.Spec.InterfaceName)
	if err != nil {
		return ctrl.Result{}, err
	}

	dt := parsers.InterfaceParserFactory{
		DeviceType: iface.Spec.DeviceType,
	}

	parser, err := dt.GetParser()
	if err != nil {
		log.Error(err, "Erro ao obter parser para o dispositivo")
		iface.Status.State = "Error"
		iface.Status.Message = err.Error()
		if err := r.Status().Update(ctx, &iface); err != nil {
			return ctrl.Result{}, err
		}
		return ctrl.Result{}, err
	}

	// Parseia a configuração atual da interface
	parsedOutput, err := parser.ParseConfig(output)

	intstatuscmd, err := device.SendCommand("show interface " + iface.Spec.InterfaceName + " | in line")
	if err != nil {
		log.Error(err, "Erro ao obter status da interface, considerando UP")
	}
	log.Info("Status da interface: ", "intstatuscmd", intstatuscmd)
	// Parseia o status da interface
	intstatus, err := parser.ParseStatus(intstatuscmd)
	if err != nil {
		log.Error(err, "Erro ao parsear status da interface")
		iface.Status.State = "Error"
		iface.Status.Message = err.Error()
		if err := r.Status().Update(ctx, &iface); err != nil {
			return ctrl.Result{}, err
		}
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
			log.Error(err, "Erro ao aplicar configuração")
			iface.Status.State = "Error"
			iface.Status.Message = err.Error()
			if err := r.Status().Update(ctx, &iface); err != nil {
				return ctrl.Result{}, err
			}
			return ctrl.Result{}, err
		}
		log.Info("Configuração aplicada com sucesso", "commands", cmds)
	} else {
		log.Info("Configuração já está atualizada")
		iface.Status.State = "Success"
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
