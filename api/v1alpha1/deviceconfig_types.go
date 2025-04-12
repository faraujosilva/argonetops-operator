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

package v1alpha1

import (
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// EDIT THIS FILE!  THIS IS SCAFFOLDING FOR YOU TO OWN!
// NOTE: json tags are required.  Any new fields you add must have json tags for the fields to be serialized.

// DeviceConfigSpec defines the desired state of DeviceConfig.
type DeviceConfigSpec struct {
	Hostname   string `json:"hostname"`
	IP         string `json:"ip"`
	DeviceType string `json:"deviceType"`
	Port       uint8  `json:"port"`
	Access     string `json:"access"`
	// Username allows referencing a ConfigMap or Secret for the username
	// +kubebuilder:validation:Optional
	Username ValueFromSource `json:"username"`

	// Password allows referencing a ConfigMap or Secret for the password
	// +kubebuilder:validation:Optional
	Password ValueFromSource `json:"password"`
}

// ValueFromSource defines a reference to a ConfigMap or Secret
type ValueFromSource struct {
	// ConfigMapKeyRef references a key in a ConfigMap
	// +kubebuilder:validation:Optional
	ConfigMapKeyRef *ConfigMapKeySelector `json:"configMapKeyRef,omitempty"`

	// SecretKeyRef references a key in a Secret
	// +kubebuilder:validation:Optional
	SecretKeyRef *SecretKeySelector `json:"secretKeyRef,omitempty"`
}

// ConfigMapKeySelector selects a key from a ConfigMap
type ConfigMapKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// SecretKeySelector selects a key from a Secret
type SecretKeySelector struct {
	Name string `json:"name"`
	Key  string `json:"key"`
}

// DeviceConfigStatus defines the observed state of DeviceConfig.
type DeviceConfigStatus struct {
	// INSERT ADDITIONAL STATUS FIELD - define observed state of cluster
	// Important: Run "make" to regenerate code after modifying this file
	State   string `json:"state,omitempty"`
	Message string `json:"message,omitempty"`
}

// +kubebuilder:object:root=true
// +kubebuilder:subresource:status

// DeviceConfig is the Schema for the deviceconfigs API.
type DeviceConfig struct {
	metav1.TypeMeta   `json:",inline"`
	metav1.ObjectMeta `json:"metadata,omitempty"`

	Spec   DeviceConfigSpec   `json:"spec,omitempty"`
	Status DeviceConfigStatus `json:"status,omitempty"`
}

// +kubebuilder:object:root=true

// DeviceConfigList contains a list of DeviceConfig.
type DeviceConfigList struct {
	metav1.TypeMeta `json:",inline"`
	metav1.ListMeta `json:"metadata,omitempty"`
	Items           []DeviceConfig `json:"items"`
}

func init() {
	SchemeBuilder.Register(&DeviceConfig{}, &DeviceConfigList{})
}
