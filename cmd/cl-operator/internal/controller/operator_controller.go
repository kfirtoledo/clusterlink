/*
Copyright 2024.

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
	"reflect"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/handler"

	cpapp "github.com/clusterlink-net/clusterlink/cmd/cl-controlplane/app"
	dpapp "github.com/clusterlink-net/clusterlink/cmd/cl-dataplane/app"
	v1api "github.com/clusterlink-net/clusterlink/cmd/cl-operator/api/v1alpha1"
	cpapi "github.com/clusterlink-net/clusterlink/pkg/controlplane/api"
	dpapi "github.com/clusterlink-net/clusterlink/pkg/dataplane/api"
	"github.com/sirupsen/logrus"
)

const (
	ControlPlaneName     = "cl-controlplane"
	DataPlaneName        = "cl-dataplane"
	IngressName          = "cl-external-svc"
	OperatorNameSpace    = "clusterlink-operator-ns"
	ClusterLinkNamespace = "clusterlink-system-ns"
	FinalizerName        = "operator.clusterlink.net/finalizer"
	ExternalPort         = 443
	ExternalNodePort     = 30443
)

// OperatorReconciler reconciles a Operator object
type OperatorReconciler struct {
	client.Client
	Scheme      *runtime.Scheme
	operator    *v1api.Operator
	clNamespace string
	Logger      *logrus.Entry
	CaFabric    []byte
}

// +kubebuilder:rbac:groups=clusterlink.net,resources=operators,verbs=get;list;watch;create;update;patch;delete
// +kubebuilder:rbac:groups=clusterlink.net,resources=operators/status,verbs=get;update;patch
// +kubebuilder:rbac:groups=clusterlink.net,resources=operators/finalizers,verbs=update
// +kubebuilder:rbac:groups="",resources=persistentvolumeclaims,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="",resources=services;pods;endpoints;namespaces,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="apps",resources=deployments,verbs=list;get;watch;create;update;patch;delete
// +kubebuilder:rbac:groups="rbac.authorization.k8s.io",resources=clusterroles;clusterrolebindings,verbs=list;get;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=clusterlink.net,resources=operators,verbs=get;list;watch;create;update;patch;delete

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the Operator object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.16.3/pkg/reconcile
func (r *OperatorReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	// Fetch the Clusterlink instance
	instance := &v1api.Operator{}
	err := r.Get(ctx, req.NamespacedName, instance)
	if err != nil {
		if errors.IsNotFound(err) {
			return ctrl.Result{}, nil
		}
		// Requeue the request on error
		return ctrl.Result{}, err
	}

	// CRD details
	r.Logger.Infof("Enter operator Reconcile - %v", instance)
	r.Logger.Infof("Enter operator Reconcile - (Namespace: %s, Name: %s)", instance.Namespace, instance.Name)
	r.Logger.Info("OperatorSpec- ",
		" DataPlane.Type: ", instance.Spec.DataPlane.Type,
		", DataPlane.Replicas: ", instance.Spec.DataPlane.Replicas,
		", Ingress.Type: ", instance.Spec.Ingress.Type,
		", LogLevel: ", instance.Spec.LogLevel,
		", ContainerRegistry: ", instance.Spec.ContainerRegistry,
		", ImageTag: ", instance.Spec.ImageTag,
	)
	r.operator = instance
	if r.operator.Spec.ContainerRegistry != "" && r.operator.Spec.ContainerRegistry[len(r.operator.Spec.ContainerRegistry)-1:] != "/" {
		r.operator.Spec.ContainerRegistry += "/"
	}

	// Set the deployment namespace
	r.clNamespace = ClusterLinkNamespace
	if r.operator.Namespace != OperatorNameSpace { // Allow to create in any namespace. TODO -should remove this part and use only ClusterLinkNamespace
		r.clNamespace = r.operator.Namespace
	}

	if r.operator.DeletionTimestamp.IsZero() {
		if err := r.setFinalizer(ctx); err != nil {
			return ctrl.Result{}, err
		}
	} else {
		if err := r.deleteAllResources(ctx); err != nil {
			return ctrl.Result{}, err
		}

		if err := r.deleteFinalizer(ctx); err != nil {
			return ctrl.Result{}, err
		}
	}

	if err := r.checkStatus(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if err := r.setClusterlink(ctx); err != nil {
		return ctrl.Result{}, err
	}

	if r.operator.Status.Controlplane.Status == v1api.StatusUpdate || r.operator.Status.Dataplane.Status == v1api.StatusUpdate {
		return ctrl.Result{Requeue: true, RequeueAfter: time.Second * 5}, err
	}
	return ctrl.Result{}, nil
}

// SetupWithManager sets up the controller with the Manager.
func (r *OperatorReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&v1api.Operator{}).
		Watches(
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: ControlPlaneName,
				},
			},
			&handler.EnqueueRequestForObject{},
		).
		Watches(
			&appsv1.Deployment{
				ObjectMeta: metav1.ObjectMeta{
					Name: DataPlaneName,
				},
			},
			&handler.EnqueueRequestForObject{},
		).
		Complete(r)
}

func (r *OperatorReconciler) setClusterlink(ctx context.Context) error {

	// Set Namespace
	if err := r.setNameSpace(ctx); err != nil {
		return err
	}

	// Set PVC
	if err := r.setPVC(ctx, ControlPlaneName); err != nil {
		return err
	}

	// Set services
	if err := r.setService(ctx, ControlPlaneName, cpapi.ListenPort); err != nil {
		return err
	}
	if err := r.setService(ctx, DataPlaneName, dpapi.ListenPort); err != nil {
		return err
	}

	// Set deployments
	if err := r.setControlplane(ctx); err != nil {
		return err
	}
	if err := r.setDataplane(ctx); err != nil {
		return err
	}

	// Set rules
	if err := r.setRules(ctx, ControlPlaneName); err != nil {
		return err
	}

	// Set External service
	if err := r.setExternalService(ctx); err != nil {
		return err
	}

	return nil
}

func (r *OperatorReconciler) setControlplane(ctx context.Context) error {
	cpDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      ControlPlaneName,
			Namespace: r.clNamespace,
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
							Image:           r.operator.Spec.ContainerRegistry + ControlPlaneName + ":" + r.operator.Spec.ImageTag,
							ImagePullPolicy: corev1.PullIfNotPresent,
							Args:            []string{"--log-level", r.operator.Spec.LogLevel, "--platform", "k8s"},
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

	return r.createOrUpdateResource(ctx, cpDeployment)
}

func (r *OperatorReconciler) setDataplane(ctx context.Context) error {
	dpDeployment := &appsv1.Deployment{
		ObjectMeta: metav1.ObjectMeta{
			Name:      DataPlaneName,
			Namespace: r.clNamespace,
		},
		Spec: appsv1.DeploymentSpec{
			Replicas: int32Ptr(int32(r.operator.Spec.DataPlane.Replicas)),
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
							Image: r.operator.Spec.ContainerRegistry + DataPlaneName + ":" + r.operator.Spec.ImageTag,
							Args: []string{
								"--log-level", r.operator.Spec.LogLevel,
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

	return r.createOrUpdateResource(ctx, dpDeployment)
}

func (r *OperatorReconciler) setService(ctx context.Context, name string, port uint16) error {
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{Name: name, Namespace: r.clNamespace},
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

	return r.createResource(ctx, service)

}

func (r *OperatorReconciler) setPVC(ctx context.Context, name string) error {
	// Create the PVC for cl-controlplane
	controlplanePVC := &corev1.PersistentVolumeClaim{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.clNamespace,
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
func (r *OperatorReconciler) setRules(ctx context.Context, name string) error {
	// Create or update the ClusterRole for cl-controlplane
	clusterRole := &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: r.clNamespace,
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
			Namespace: r.clNamespace,
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

func (r *OperatorReconciler) setNameSpace(ctx context.Context) error {
	ns := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: r.clNamespace,
		},
	}

	return r.createResource(ctx, ns)
}

func (r *OperatorReconciler) setExternalService(ctx context.Context) error {

	// Create a Service object
	service := &corev1.Service{
		ObjectMeta: metav1.ObjectMeta{
			Name:      IngressName,
			Namespace: r.clNamespace,
		},
		Spec: corev1.ServiceSpec{
			Selector: map[string]string{
				"app": "cl-dataplane",
			},
			Ports: []corev1.ServicePort{
				{
					Port:       ExternalPort,
					TargetPort: intstr.FromInt(ExternalPort),
					Protocol:   corev1.ProtocolTCP,
					Name:       "http",
				},
			},
		},
	}

	switch r.operator.Spec.Ingress.Type {
	case v1api.IngressTypeNone:
		return nil
	case v1api.IngressTypeNodePort:
		service.Spec.Type = corev1.ServiceTypeNodePort
		service.Spec.Ports[0].NodePort = ExternalNodePort
	case v1api.IngressTypeLB:
		service.Spec.Type = corev1.ServiceTypeLoadBalancer
	}

	return r.createResource(ctx, service)
}

func (r *OperatorReconciler) setFinalizer(ctx context.Context) error {
	if !controllerutil.ContainsFinalizer(r.operator, FinalizerName) {
		controllerutil.AddFinalizer(r.operator, FinalizerName)
		return r.Update(ctx, r.operator)
	}

	return nil
}

func (r *OperatorReconciler) deleteFinalizer(ctx context.Context) error {
	// remove our finalizer from the list and update it.
	controllerutil.RemoveFinalizer(r.operator, FinalizerName)
	return r.Update(ctx, r.operator)
}

func (r *OperatorReconciler) createOrUpdateResource(ctx context.Context, object client.Object) error {
	// Use CreateOrUpdate to create or update the PVC for cl-controlplane
	_, err := controllerutil.CreateOrUpdate(ctx, r.Client, object, func() error {
		createTime := object.GetCreationTimestamp()
		if createTime.IsZero() {
			r.Logger.Infof("Create resource %v Name: %s Namespace: %s", reflect.TypeOf(object), object.GetName(), object.GetNamespace())
		}
		return nil
	})

	if err != nil {
		r.Logger.Error(err)
	}
	return err
}

func (r *OperatorReconciler) createResource(ctx context.Context, object client.Object) error {
	err := r.Get(ctx, types.NamespacedName{Name: object.GetName(), Namespace: object.GetNamespace()}, object)
	if err != nil {
		if client.IgnoreNotFound(err) != nil {
			return err
		}

		r.Logger.Infof("Create resource %s Name: %s Namespace: %s", reflect.TypeOf(object), object.GetName(), object.GetNamespace())
		return r.Create(ctx, object)
	}

	return nil
}

func (r *OperatorReconciler) deleteAllResources(ctx context.Context) error {
	// Delete deployments
	if err := r.deleteResource(ctx, DataPlaneName, &appsv1.Deployment{}); err != nil {
		return err
	}

	if err := r.deleteResource(ctx, ControlPlaneName, &appsv1.Deployment{}); err != nil {
		return err
	}
	// Delete services
	if err := r.deleteResource(ctx, DataPlaneName, &corev1.Service{}); err != nil {
		return err
	}

	if err := r.deleteResource(ctx, ControlPlaneName, &corev1.Service{}); err != nil {
		return err
	}

	// Delete PVC
	if err := r.deleteResource(ctx, ControlPlaneName, &corev1.PersistentVolumeClaim{}); err != nil {
		return err
	}

	// Delete rules
	if err := r.deleteResource(ctx, ControlPlaneName, &rbacv1.ClusterRole{}); err != nil {
		return err
	}

	if err := r.deleteResource(ctx, ControlPlaneName, &rbacv1.ClusterRoleBinding{}); err != nil {
		return err
	}

	// Delete ingress
	if err := r.deleteResource(ctx, IngressName, &corev1.Service{}); err != nil {
		return err
	}

	if r.clNamespace == ClusterLinkNamespace {
		if err := r.deleteResource(ctx, ControlPlaneName, &corev1.Namespace{}); err != nil {
			return err
		}
	}

	return nil

}

func (r *OperatorReconciler) deleteResource(ctx context.Context, name string, object client.Object) error {
	err := r.Get(ctx, types.NamespacedName{Name: name, Namespace: r.clNamespace}, object)
	if client.IgnoreNotFound(err) != nil {
		return err
	}

	return r.Delete(ctx, object)
}

// Helper function to convert int32 to *int32
func int32Ptr(i int32) *int32 {
	return &i
}

func (r *OperatorReconciler) checkStatus(ctx context.Context) error {
	// Delete ingress
	if err := r.checkControlplaneStatus(ctx); err != nil {
		return err
	}
	return r.checkDataplaneStatus(ctx)
}

func (r *OperatorReconciler) checkControlplaneStatus(ctx context.Context) error {
	cpStatus, msg, err := r.checkDeploymnetStatus(ctx, types.NamespacedName{Name: ControlPlaneName, Namespace: r.clNamespace})
	if err != nil {
		return err
	}

	if r.operator.Status.Controlplane.Status != cpStatus {
		r.Logger.Debugf("Change controlPlaneStatus froms %s to %s", r.operator.Status.Controlplane.Status, cpStatus)
		r.operator.Status.Controlplane.Status = cpStatus
		if cpStatus == v1api.StatusError {
			r.operator.Status.Controlplane.Message = msg
		}
		return r.Status().Update(ctx, r.operator)
	}

	return nil
}

func (r *OperatorReconciler) checkDataplaneStatus(ctx context.Context) error {
	dpStatus, msg, err := r.checkDeploymnetStatus(ctx, types.NamespacedName{Name: DataPlaneName, Namespace: r.clNamespace})
	if err != nil {
		return err
	}
	if r.operator.Status.Dataplane.Status != dpStatus {
		r.Logger.Debugf("Change DataplaneStatus froms %s to %s", r.operator.Status.Dataplane.Status, dpStatus)
		r.operator.Status.Dataplane.Status = dpStatus
		if dpStatus == v1api.StatusError {
			r.operator.Status.Dataplane.Message = msg
		}
		return r.Status().Update(ctx, r.operator)
	}

	return nil
}

func (r *OperatorReconciler) checkDeploymnetStatus(ctx context.Context, name types.NamespacedName) (v1api.StatusType, string, error) {
	d := &appsv1.Deployment{}
	status := v1api.StatusNone
	if err := r.Get(ctx, name, d); err != nil {
		if errors.IsNotFound(err) {
			return status, "", nil
		}
		return "", "", err
	}

	// Check the conditions in the updated Deployment status
	conditions := d.Status.Conditions
	for _, condition := range conditions {
		switch condition.Type {
		case appsv1.DeploymentAvailable:
			if condition.Status == corev1.ConditionTrue {
				return v1api.StatusReady, "", nil
			} else {
				status = v1api.StatusUpdate
			}
		case appsv1.DeploymentProgressing, appsv1.DeploymentReplicaFailure:
			if condition.Status != corev1.ConditionTrue {
				return v1api.StatusError, condition.Message, nil
			}
		}

	}
	return status, "", nil
}
