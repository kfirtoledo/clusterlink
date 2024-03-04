// Copyright 2023 The ClusterLink Authors.
// Licensed under the Apache License, Version 2.0 (the "License");
// you may not use this file except in compliance with the License.
// You may obtain a copy of the License at
//
//     http://www.apache.org/licenses/LICENSE-2.0
//
// Unless required by applicable law or agreed to in writing, software
// distributed under the License is distributed on an "AS IS" BASIS,
// WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
// See the License for the specific language governing permissions and
// limitations under the License.

package rest

import (
	"context"
	"encoding/json"
	"fmt"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"

	"github.com/clusterlink-net/clusterlink/pkg/api"
	"github.com/clusterlink-net/clusterlink/pkg/apis/clusterlink.net/v1alpha1"
	"github.com/clusterlink-net/clusterlink/pkg/controlplane/store"
)

type importHandler struct {
	manager *Manager
}

func toK8SImport(imp *store.Import, namespace string) *v1alpha1.Import {
	return &v1alpha1.Import{
		ObjectMeta: metav1.ObjectMeta{
			Name:      imp.Name,
			Namespace: namespace,
		},
		Spec: v1alpha1.ImportSpec{
			Port:       imp.ImportSpec.Service.Port,
			TargetPort: imp.Port,
		},
	}
}

func importToAPI(imp *store.Import) *api.Import {
	if imp == nil {
		return nil
	}

	return &api.Import{
		Name: imp.Name,
		Spec: imp.ImportSpec,
		Status: api.ImportStatus{
			Listener: api.Endpoint{ // Endpoint.Host is not set
				Port: imp.Port,
			},
		},
	}
}

// CreateImport creates a listening socket for an imported remote service.
func (m *Manager) CreateImport(imp *store.Import) error {
	m.logger.Infof("Creating import '%s'.", imp.Name)

	k8sImp := toK8SImport(imp, m.namespace)

	if m.initialized {
		if err := m.imports.Create(imp); err != nil {
			return err
		}

		err := m.controlManager.AddImport(context.Background(), k8sImp)
		if err != nil {
			return err
		}

		imp.Port = k8sImp.Spec.TargetPort

		err = m.imports.Update(imp.Name, func(old *store.Import) *store.Import {
			return imp
		})
		if err != nil {
			return err
		}
	}

	if err := m.xdsManager.AddImport(k8sImp); err != nil {
		// practically impossible
		return err
	}

	return nil
}

// UpdateImport updates a listening socket for an imported remote service.
func (m *Manager) UpdateImport(imp *store.Import) error {
	m.logger.Infof("Updating import '%s'.", imp.Name)

	err := m.imports.Update(imp.Name, func(old *store.Import) *store.Import {
		return imp
	})
	if err != nil {
		return err
	}

	k8sImp := toK8SImport(imp, m.namespace)
	err = m.controlManager.AddImport(context.Background(), k8sImp)
	if err != nil {
		return err
	}

	imp.Port = k8sImp.Spec.TargetPort

	err = m.imports.Update(imp.Name, func(old *store.Import) *store.Import {
		return imp
	})
	if err != nil {
		return err
	}

	if err := m.xdsManager.AddImport(k8sImp); err != nil {
		// practically impossible
		return err
	}

	return nil
}

// GetImport returns an existing import.
func (m *Manager) GetImport(name string) *store.Import {
	m.logger.Infof("Getting import '%s'.", name)
	return m.imports.Get(name)
}

// DeleteImport removes the listening socket of a previously imported service.
func (m *Manager) DeleteImport(name string) (*store.Import, error) {
	m.logger.Infof("Deleting import '%s'.", name)

	imp, err := m.imports.Delete(name)
	if err != nil {
		return nil, err
	}
	if imp == nil {
		return nil, nil
	}

	namespacedName := types.NamespacedName{
		Name:      name,
		Namespace: m.namespace,
	}
	if err := m.xdsManager.DeleteImport(namespacedName); err != nil {
		// practically impossible
		return imp, err
	}

	err = m.controlManager.DeleteImport(
		context.Background(),
		toK8SImport(imp, m.namespace))
	if err != nil {
		return nil, err
	}

	return imp, nil
}

// GetAllImports returns the list of all imports.
func (m *Manager) GetAllImports() []*store.Import {
	m.logger.Info("Listing all imports.")
	return m.imports.GetAll()
}

// Decode an import.
func (h *importHandler) Decode(data []byte) (any, error) {
	var imp api.Import
	if err := json.Unmarshal(data, &imp); err != nil {
		return nil, fmt.Errorf("cannot decode import: %w", err)
	}

	if imp.Name == "" {
		return nil, fmt.Errorf("empty import name")
	}

	if imp.Spec.Service.Host == "" {
		return nil, fmt.Errorf("missing service name")
	}

	if imp.Spec.Service.Port == 0 {
		return nil, fmt.Errorf("missing service port")
	}

	return store.NewImport(&imp), nil
}

// Create an import.
func (h *importHandler) Create(object any) error {
	return h.manager.CreateImport(object.(*store.Import))
}

// Update an import.
func (h *importHandler) Update(object any) error {
	return h.manager.UpdateImport(object.(*store.Import))
}

// Get an import.
func (h *importHandler) Get(name string) (any, error) {
	imp := importToAPI(h.manager.GetImport(name))
	if imp == nil {
		return nil, nil
	}
	return imp, nil
}

// Delete an import.
func (h *importHandler) Delete(name any) (any, error) {
	return h.manager.DeleteImport(name.(string))
}

// List all imports.
func (h *importHandler) List() (any, error) {
	imports := h.manager.GetAllImports()
	apiImports := make([]*api.Import, len(imports))
	for i, imp := range imports {
		apiImports[i] = importToAPI(imp)
	}
	return apiImports, nil
}
