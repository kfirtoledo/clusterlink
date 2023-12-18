/*
Copyright 2023.

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
	"path/filepath"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"

	cpapp "github.com/clusterlink-net/clusterlink/cmd/cl-controlplane/app"
	dpapp "github.com/clusterlink-net/clusterlink/cmd/cl-dataplane/app"
	clv1 "github.com/clusterlink-net/clusterlink/cmd/cl-operator/api/v1"
	cpapi "github.com/clusterlink-net/clusterlink/pkg/controlplane/api"
	dpapi "github.com/clusterlink-net/clusterlink/pkg/dataplane/api"

	"github.com/sirupsen/logrus"
)

const (
	ControlPlaneName = "cl-controlplane"
	DataPlaneName    = "cl-dataplane"
)

// ClusterlinkReconciler reconciles a Clusterlink object
type ClusterlinkReconciler struct {
	client.Client
	Scheme   *runtime.Scheme
	clCR     *clv1.Clusterlink
	Logger   *logrus.Entry
	CaFabric []byte
}

// +kubebuilder:rbac:groups=cl.clusterlink.net,resources=clusterlinks,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=cl.clusterlink.net,resources=clusterlinks/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=cl.clusterlink.net,resources=clusterlinks/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;pods;endpoints,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles;clusterrolebindings,verbs=list;get;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Clusterlink object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *ClusterlinkReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the Clusterlink instance
	instance := &clv1.Clusterlink{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Requeue the request on error
		return ctrl.Result{}, err
	}

	// CRD details
	r.Logger.Info("Reconciling Clusterlink", "Namespace", instance.Namespace, "Name", instance.Name)
	r.Logger.Info("ClusterlinkSpec",
		"DataPlane.Type", instance.Spec.DataPlane.Type,
		"DataPlane.Replicates", instance.Spec.DataPlane.Replicates,
		"LogLevel", instance.Spec.LogLevel,
		"ContainerRegistry", instance.Spec.ContainerRegistry,
		"ImageTag", instance.Spec.ImageTag,
	)
	r.clCR = instance
	if err := r.createClusterlink(ctx); err != nil {
		return ctrl.Result{}, nil
	}

	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *ClusterlinkReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clv1.Clusterlink{}).
		Complete(r)
}
func (r *ClusterlinkReconciler) createClusterlink(ctx context.Context) error {

	r.Logger.Info("Start create clusterlink Deployments")
	// Create PVC
	r.createPVC(ctx, ControlPlaneName)

	// Create services
	r.createService(ctx, ControlPlaneName, cpapi.ListenPort)
	r.createService(ctx, DataPlaneName, dpapi.ListenPort)

	// Create deployments
	r.createControlplane(ctx)
	r.createDataplane(ctx)

	// Create rules
	r.createRules(ctx, ControlPlaneName)
	return nil
}

func (r *ClusterlinkReconciler) createControlplane(ctx context.Context) error {
	controlplaneDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControlPlaneName,
			Namespace: r.clCR.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(1),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": ControlPlaneName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": ControlPlaneName},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "ca",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "cl-fabric",
								},
							},
						},
						{
							Name: "tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: ControlPlaneName,
								},
							},
						},
						{
							Name: ControlPlaneName,
							VolumeSource: corev1.VolumeSource{
								PersistentVolumeClaim: &corev1.PersistentVolumeClaimVolumeSource{
									ClaimName: ControlPlaneName,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:            ControlPlaneName,
							Image:           r.clCR.Spec.ContainerRegistry + ControlPlaneName + ":" + r.clCR.Spec.ImageTag,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"--log-level", r.clCR.Spec.LogLevel, "--platform", "k8s"},
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: cpapi.ListenPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ca",
									MountPath: cpapp.CAFile,
									SubPath:   "ca",
									ReadOnly:  true,
								},
								{
									Name:      "tls",
									MountPath: cpapp.CertificateFile,
									SubPath:   "cert",
									ReadOnly:  true,
								},
								{
									Name:      "tls",
									MountPath: cpapp.KeyFile,
									SubPath:   "key",
									ReadOnly:  true,
								},
								{
									Name:      ControlPlaneName,
									MountPath: filepath.Dir(cpapp.StoreFile),
								},
							},
							Env: []corev1.EnvVar{
								{
									Name: "CL-NAMESPACE",
									ValueFrom: &corev1.EnvVarSource{
										FieldRef: &corev1.ObjectFieldSelector{
											FieldPath: "metadata.namespace",
										},
									},
								},
							},
						},
					},
				},
			},
		},
	}
	return r.createResource(ctx, controlplaneDeployment)
}

func (r *ClusterlinkReconciler) createDataplane(ctx context.Context) error {
	dataplaneDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DataPlaneName,
			Namespace: r.clCR.Namespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(int32(r.clCR.Spec.DataPlane.Replicates)),
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{"app": DataPlaneName},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Labels: map[string]string{"app": DataPlaneName},
				},
				Spec: corev1.PodSpec{
					Volumes: []corev1.Volume{
						{
							Name: "ca",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: "cl-fabric",
								},
							},
						},
						{
							Name: "tls",
							VolumeSource: corev1.VolumeSource{
								Secret: &corev1.SecretVolumeSource{
									SecretName: DataPlaneName,
								},
							},
						},
					},
					Containers: []corev1.Container{
						{
							Name:  "dataplane",
							Image: r.clCR.Spec.ContainerRegistry + DataPlaneName + ":" + r.clCR.Spec.ImageTag,
							Args: []string{
								"--log-level", r.clCR.Spec.LogLevel,
								"--controlplane-host", ControlPlaneName,
							},
							ImagePullPolicy: corev1.PullIfNotPresent,
							Ports: []corev1.ContainerPort{
								{
									ContainerPort: dpapi.ListenPort,
								},
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									Name:      "ca",
									MountPath: dpapp.CAFile,
									SubPath:   "ca",
									ReadOnly:  true,
								},
								{
									Name:      "tls",
									MountPath: dpapp.CertificateFile,
									SubPath:   "cert",
									ReadOnly:  true,
								},
								{
									Name:      "tls",
									MountPath: dpapp.KeyFile,
									SubPath:   "key",
									ReadOnly:  true,
								},
							},
						},
					},
				},
			},
		},
	}
	return r.createResource(ctx, dataplaneDeployment)
}

func (r *ClusterlinkReconciler) createService(ctx context.Context, name string, port uint16) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.clCR.Namespace},
		Spec: corev1.ServiceSpec{
			Ports: []corev1.ServicePort{
				{
					Protocol: corev1.ProtocolTCP,
					Port:     int32(port),
				},
			},
			Type:     corev1.ServiceTypeClusterIP,
			Selector: map[string]string{"app": name},
		},
	}
	// Set the owner reference to link the secret to the Custom Resource
	if err := ctrl.SetControllerReference(r.clCR, service, r.Scheme); err != nil {
		return err
	}
	// Check if the secret already exists
	existingService := &corev1.Service{}
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: r.clCR.Namespace}, existingService)
	if err != nil && errors.IsNotFound(err) {
		// Secret doesn't exist, create it
		if err := r.Create(ctx, service); err != nil {
			return err
		}
		r.Logger.Info("service created", "Name", service.Name, "Namespace", service.Namespace)
	} else if err == nil {
		r.Logger.Info("service already exist", "Name", existingService.Name, "Namespace", existingService.Namespace)
	} else {
		return err
	}
	return nil
}

func (r *ClusterlinkReconciler) createSecret(ctx context.Context, name string, secretData map[string][]byte) error {
	// Create a new Secret object
	secret := &corev1.Secret{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.clCR.Namespace,
		},
		Data: secretData,
	}

	return r.createResource(ctx, secret)

}

func (r *ClusterlinkReconciler) createPVC(ctx context.Context, name string) error {
	// Create the PVC for cl-controlplane
	controlplanePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.clCR.Namespace,
		},
		Spec: corev1.PersistentVolumeClaimSpec{
			AccessModes: []corev1.PersistentVolumeAccessMode{corev1.ReadWriteOnce},
			Resources: corev1.ResourceRequirements{
				Requests: corev1.ResourceList{
					corev1.ResourceStorage: resource.MustParse("100Mi"),
				},
			},
		},
	}
	return r.createResource(ctx, controlplanePVC)

}
func (r *ClusterlinkReconciler) createRules(ctx context.Context, name string) error {
	// Create or update the ClusterRole for cl-controlplane
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.clCR.Namespace,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups: []string{""},
				Resources: []string{"services", "endpoints"},
				Verbs:     []string{"create", "delete", "update"},
			},
			{
				APIGroups: []string{""},
				Resources: []string{"pods"},
				Verbs:     []string{"get", "list", "watch"},
			},
		},
	}
	err := r.createResource(ctx, clusterRole)
	if err != nil {
		return err
	}
	// Create or update the ClusterRoleBinding for cl-controlplane
	clusterRoleBinding := &rbacv1.ClusterRoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.clCR.Namespace,
		},
		RoleRef: rbacv1.RoleRef{
			APIGroup: "rbac.authorization.k8s.io",
			Kind:     "ClusterRole",
			Name:     name,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      "default",
				Namespace: "default",
			},
		},
	}
	return r.createResource(ctx, clusterRoleBinding)

}
func (r *ClusterlinkReconciler) createResource(ctx context.Context, object client.Object) error {
	r.Logger.Infof("Create resource %s %s", object.GetObjectKind(), object.GetName())
	// Set the owner reference to link the secret to the Custom Resource
	if err := ctrl.SetControllerReference(r.clCR, object, r.Scheme); err != nil {
		r.Logger.Error(err)
		return err
	}

	// Use CreateOrUpdate to create or update the PVC for cl-controlplane
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, object, func() error {
		// Set any fields you want to modify before the update
		return nil
	})
	if err != nil {
		r.Logger.Error(err)
	}
	return err
}

// Helper function to convert int32 to *int32
func int32Ptr(i int32) *int32 {
	return &i
}
