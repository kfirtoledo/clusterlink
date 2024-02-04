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

package controller_test

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"testing"
	"time"

	. "github.com/onsi/gomega"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes/scheme"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	clusterlink "github.com/clusterlink-net/clusterlink/pkg/apis/clusterlink.net/v1alpha1"
	"github.com/clusterlink-net/clusterlink/pkg/operator/controller"
)

const (
	ClusterLinkName = "cl-job"
	Timeout         = time.Second * 30
	duration        = time.Second * 10
	IntervalTime    = time.Millisecond * 250
)

var (
	k8sClient client.Client
	ctx       context.Context
	cancel    context.CancelFunc
)

func TestMain(m *testing.M) {
	// Bootstrap test environment
	ctx, cancel = context.WithCancel(context.TODO())
	testEnv := &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "..", "config", "operator", "crds")},
		ErrorIfCRDPathMissing: true,
		BinaryAssetsDirectory: filepath.Join("..", "..", "..", "bin", "k8s",
			fmt.Sprintf("1.28.3-%s-%s", runtime.GOOS, runtime.GOARCH)),
	}

	var err error
	cfg, err := testEnv.Start()
	if err != nil {
		fmt.Printf("Failed to start test environment: %v\n", err)
		cancel()
		return
	}

	err = clusterlink.AddToScheme(scheme.Scheme)
	if err != nil {
		fmt.Printf("Failed to add API to scheme: %v\n", err)
		cancel()
		return
	}

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	if err != nil {
		fmt.Printf("Failed to create Kubernetes client: %v\n", err)
		cancel()
		return
	}

	k8sManager, err := ctrl.NewManager(cfg, ctrl.Options{
		Scheme: scheme.Scheme,
	})
	if err != nil {
		fmt.Printf("Failed to create controller manager: %v\n", err)
		cancel()
		return
	}

	err = (&controller.InstanceReconciler{
		Client:        k8sManager.GetClient(),
		Scheme:        k8sManager.GetScheme(),
		Logger:        logrus.WithField("component", "reconciler"),
		InstancesMeta: make(map[string]string),
	}).SetupWithManager(k8sManager)
	if err != nil {
		fmt.Printf("Failed to set up reconciler: %v\n", err)
		cancel()
		return
	}

	go func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Printf("Recovered from panic: %v\n", r)
			}
		}()
		err := k8sManager.Start(ctx)
		if err != nil {
			fmt.Printf("Failed to run controller manager: %v\n", err)
		}
	}()

	code := m.Run()

	// Cancelling the context
	cancel()
	// Tearing down the test environment
	err = testEnv.Stop()
	if err != nil {
		fmt.Printf("Failed to stop test environment: %v\n", err)
	}

	// Exiting with the status code from the test run
	os.Exit(code)
}

func TestClusterLinkController(t *testing.T) {
	RegisterTestingT(t) // Setup gomega
	// Define utility constants for object names and testing timeouts/durations and intervals.

	cl := clusterlink.Instance{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch.tutorial.kubebuilder.io/v1",
			Kind:       "Clusterlink",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterLinkName,
			Namespace: controller.OperatorNameSpace,
		},
	}

	cpID := types.NamespacedName{Name: controller.ControlPlaneName, Namespace: controller.InstanceNamespace}
	cpResource := []client.Object{
		&appsv1.Deployment{}, &corev1.Service{}, &corev1.PersistentVolumeClaim{},
		&rbacv1.ClusterRole{}, &rbacv1.ClusterRoleBinding{},
	}
	dpID := types.NamespacedName{Name: controller.DataPlaneName, Namespace: controller.InstanceNamespace}
	dpResource := []client.Object{&appsv1.Deployment{}, &corev1.Service{}}
	ingressID := types.NamespacedName{Name: controller.IngressName, Namespace: controller.InstanceNamespace}

	t.Run("Create ClusterLink deployment", func(t *testing.T) {
		// Create ClusterLink namespaces
		opearatorNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: controller.OperatorNameSpace,
			},
		}
		Expect(k8sClient.Create(ctx, opearatorNamespace)).Should(Succeed())

		systemNamespace := &corev1.Namespace{
			ObjectMeta: metav1.ObjectMeta{
				Name: controller.InstanceNamespace,
			},
		}
		Expect(k8sClient.Create(ctx, systemNamespace)).Should(Succeed())

		// Create ClusterLink deployment
		Expect(k8sClient.Create(ctx, &cl)).Should(Succeed())

		// Check Controlplane resources
		for _, r := range cpResource {
			checkResourceCreated(cpID, r)
		}

		// Check controlplane fields
		cp := &appsv1.Deployment{}
		getResource(cpID, cp)
		cpImage := "ghcr.io/clusterlink-net/" + controller.ControlPlaneName + ":latest"
		Expect(cp.Spec.Template.Spec.Containers[0].Image).Should(Equal(cpImage))
		Expect(cp.Spec.Template.Spec.Containers[0].Args[1]).Should(Equal("info"))

		// Check Dataplane resources
		for _, r := range dpResource {
			checkResourceCreated(dpID, r)
		}

		// Check Dataplane fields
		dp := &appsv1.Deployment{}
		getResource(dpID, dp)
		envoyImage := "ghcr.io/clusterlink-net/" + controller.DataPlaneName + ":latest"
		Expect(dp.Spec.Template.Spec.Containers[0].Image).Should(Equal(envoyImage))
		Expect(*dp.Spec.Replicas).Should(Equal(int32(1)))
		Expect(dp.Spec.Template.Spec.Containers[0].Args[1]).Should(Equal("info"))

		// Check ingress resource not exist
		assert.True(t, checkResourceNotExist(ingressID, &corev1.Service{}))
	})

	t.Run("Update ClusterLink deployment", func(t *testing.T) {
		svc := &corev1.Service{}
		// Update ingress resource to LoadBalancer type
		getResource(types.NamespacedName{Name: ClusterLinkName, Namespace: controller.OperatorNameSpace}, &cl)
		cl.Spec.Ingress.Type = clusterlink.IngressTypeLoadBalancer
		Expect(k8sClient.Update(ctx, &cl)).Should(Succeed())
		checkResourceCreated(ingressID, svc)
		Expect(svc.Spec.Type).Should(Equal(corev1.ServiceTypeLoadBalancer))

		// Update ingress resource to NodPport type
		Expect(k8sClient.Delete(ctx, svc)).Should(Succeed())
		getResource(types.NamespacedName{Name: ClusterLinkName, Namespace: controller.OperatorNameSpace}, &cl)
		cl.Spec.Ingress.Type = clusterlink.IngressTypeNodePort
		cl.Spec.Ingress.Port = 30444
		Expect(k8sClient.Update(ctx, &cl)).Should(Succeed())
		checkResourceCreated(ingressID, svc)
		Expect(svc.Spec.Type).Should(Equal(corev1.ServiceTypeNodePort))
		Expect(svc.Spec.Ports[0].NodePort).Should(Equal(int32(30444)))

		// Update dataplane and controlpane
		goReplicas := 2
		loglevel := "debug"
		containerRegistry := "quay.com"
		imageTag := "v1.0.1"
		cp := &appsv1.Deployment{}
		dp := &appsv1.Deployment{}

		getResource(cpID, cp)
		Expect(k8sClient.Delete(ctx, cp)).Should(Succeed())
		getResource(dpID, dp)
		Expect(k8sClient.Delete(ctx, dp)).Should(Succeed())
		getResource(types.NamespacedName{Name: ClusterLinkName, Namespace: controller.OperatorNameSpace}, &cl)

		/// Update Spec
		cl.Spec.DataPlane.Type = clusterlink.DataplaneTypeGo
		cl.Spec.DataPlane.Replicas = goReplicas
		cl.Spec.LogLevel = loglevel
		cl.Spec.ImageTag = imageTag
		cl.Spec.ContainerRegistry = containerRegistry
		Expect(k8sClient.Update(ctx, &cl)).Should(Succeed())

		/// Check controlplane
		checkResourceCreated(cpID, cp)
		cpImage := containerRegistry + "/" + controller.ControlPlaneName + ":" + imageTag
		Expect(cp.Spec.Template.Spec.Containers[0].Image).Should(Equal(cpImage))
		Expect(cp.Spec.Template.Spec.Containers[0].Args[1]).Should(Equal(loglevel))

		/// Check dataplane
		checkResourceCreated(dpID, dp)
		goImage := containerRegistry + "/" + controller.GoDataPlaneName + ":" + imageTag
		Expect(dp.Spec.Template.Spec.Containers[0].Image).Should(Equal(goImage))
		Expect(*dp.Spec.Replicas).Should(Equal(int32(goReplicas)))
		Expect(dp.Spec.Template.Spec.Containers[0].Args[1]).Should(Equal(loglevel))
	})

	t.Run("Delete ClusterLink deployment", func(t *testing.T) {
		// Delete ClusterLink deployment
		Expect(k8sClient.Delete(ctx, &cl)).Should(Succeed())

		// Controlplane resources
		for _, r := range cpResource {
			checkResourceDeleted(cpID, r)
		}

		// Dataplane resources
		for _, r := range dpResource {
			checkResourceDeleted(dpID, r)
		}

		// Ingress resource
		checkResourceDeleted(ingressID, &corev1.Service{})
	})
}

// checkResourceCreated checks k8s resource was deleted.
func checkResourceDeleted(id types.NamespacedName, object client.Object) {
	Eventually(func() bool {
		err := k8sClient.Get(ctx, id, object)
		if err != nil {
			return apierrors.IsNotFound(err)
		}
		// Check if the resource has a deletion timestamp
		if !object.GetDeletionTimestamp().IsZero() {
			return true
		}
		// If the resource exists and has no deletion timestamp, continue waiting
		return false
	}, Timeout, IntervalTime).Should(BeTrue())
}

// checkResourceCreated checks k8s resource was created.
func checkResourceCreated(id types.NamespacedName, object client.Object) {
	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), id, object)
		return err == nil
	}, Timeout, IntervalTime).Should(BeTrue())
}

// checkResourceNotExist checks k8s resource is not existed.
func checkResourceNotExist(id types.NamespacedName, object client.Object) bool {
	err := k8sClient.Get(ctx, id, object)
	if err != nil {
		return apierrors.IsNotFound(err)
	}

	return false
}

// getResource gets the k8s resource according to the resource id.
func getResource(id types.NamespacedName, object client.Object) {
	err := k8sClient.Get(context.Background(), id, object)
	Expect(err).NotTo(HaveOccurred())
}
