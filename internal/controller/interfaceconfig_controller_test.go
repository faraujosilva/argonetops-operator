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
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/reconcile"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	networkv1alpha1 "github.com/seunome/argonetops-operator/api/v1alpha1"
)

var _ = Describe("InterfaceConfig Controller", func() {

	Context("Reconcile function", func() {
		const resourceName = "test-reconcile-resource"

		ctx := context.Background()

		typeNamespacedName := types.NamespacedName{
			Name:      resourceName,
			Namespace: "default",
		}

		BeforeEach(func() {
			By("creating the custom resource for the Kind InterfaceConfig")
			resource := &networkv1alpha1.InterfaceConfig{
				ObjectMeta: metav1.ObjectMeta{
					Name:      resourceName,
					Namespace: "default",
				},
				Spec: networkv1alpha1.InterfaceConfigSpec{
					DeviceIP:      "192.168.1.1",
					Username:      "admin",
					Password:      "password",
					DeviceType:    "cisco_ios",
					DevicePort:    22,
					InterfaceName: "GigabitEthernet0/1",
					Description:   "Test Interface",
					IPAddress:     "192.168.1.2",
					SubnetMask:    "255.255.255.0",
					Shutdown:      false,
				},
			}
			Expect(k8sClient.Create(ctx, resource)).To(Succeed())
		})

		AfterEach(func() {
			By("cleaning up the custom resource for the Kind InterfaceConfig")
			resource := &networkv1alpha1.InterfaceConfig{}
			err := k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(err).NotTo(HaveOccurred())
			Expect(k8sClient.Delete(ctx, resource)).To(Succeed())
		})

		It("should add a finalizer and reconcile successfully", func() {
			By("Reconciling the created resource")
			controllerReconciler := &InterfaceConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			result, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())
			Expect(result.RequeueAfter).To(BeNumerically(">", 0))

			By("Verifying the finalizer was added")
			resource := &networkv1alpha1.InterfaceConfig{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			Expect(controllerutil.ContainsFinalizer(resource, "network.argonetops.io/finalizer")).To(BeTrue())
		})

		It("should handle deletion and perform rollback", func() {
			By("Marking the resource for deletion")
			resource := &networkv1alpha1.InterfaceConfig{}
			Expect(k8sClient.Get(ctx, typeNamespacedName, resource)).To(Succeed())
			resource.SetDeletionTimestamp(&metav1.Time{Time: time.Now()})
			Expect(k8sClient.Update(ctx, resource)).To(Succeed())

			By("Reconciling the resource marked for deletion")
			controllerReconciler := &InterfaceConfigReconciler{
				Client: k8sClient,
				Scheme: k8sClient.Scheme(),
			}

			_, err := controllerReconciler.Reconcile(ctx, reconcile.Request{
				NamespacedName: typeNamespacedName,
			})
			Expect(err).NotTo(HaveOccurred())

			By("Verifying the finalizer was removed after rollback")
			err = k8sClient.Get(ctx, typeNamespacedName, resource)
			Expect(errors.IsNotFound(err)).To(BeTrue())
		})
	})
})
