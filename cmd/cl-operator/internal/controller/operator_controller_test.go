/*

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
// +kubebuilder:docs-gen:collapse=Apache License

/*
Ideally, we should have one `<kind>_controller_test.go` for each controller scaffolded and called in the `suite_test.go`.
So, let's write our example test for the CronJob controller (`cronjob_controller_test.go.`)
*/

/*
As usual, we start with the necessary imports. We also define some utility variables.
*/
package controller

import (
	"context"
	"fmt"
	"reflect"
	"time"

	api "github.com/clusterlink-net/clusterlink/cmd/cl-operator/api/v1alpha1"
	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	"sigs.k8s.io/controller-runtime/pkg/client"

	"k8s.io/apimachinery/pkg/types"
)

// +kubebuilder:docs-gen:collapse=Imports
const (
	ClusterLinkName = "cl-job"
	Timeout         = time.Second * 30
	duration        = time.Second * 10
	IntervalTime    = time.Millisecond * 250
)

var _ = Describe("ClusterLInk controller test", func() {

	// Define utility constants for object names and testing timeouts/durations and intervals.
	cl := api.Operator{
		TypeMeta: metav1.TypeMeta{
			APIVersion: "batch.tutorial.kubebuilder.io/v1",
			Kind:       "Clusterlink",
		},
		ObjectMeta: metav1.ObjectMeta{
			Name:      ClusterLinkName,
			Namespace: OperatorNameSpace,
		},
	}

	cpID := types.NamespacedName{Name: ControlPlaneName, Namespace: ClusterLinkNamespace}
	cpResource := []client.Object{&appsv1.Deployment{}, &corev1.Service{}, &corev1.PersistentVolumeClaim{}, &rbacv1.ClusterRole{}, &rbacv1.ClusterRoleBinding{}}
	dpID := types.NamespacedName{Name: DataPlaneName, Namespace: ClusterLinkNamespace}
	dpResource := []client.Object{&appsv1.Deployment{}, &corev1.Service{}}
	ingressID := types.NamespacedName{Name: IngressName, Namespace: ClusterLinkNamespace}

	Context("Create ClusterLink deployment", func() {
		It("Create ClusterLink operator namespace", func() {
			ns := &corev1.Namespace{
				ObjectMeta: metav1.ObjectMeta{
					Name: OperatorNameSpace,
				},
			}
			Expect(k8sClient.Create(ctx, ns)).Should(Succeed())
		})
		It("Should create ClusterLink deployment", func() {
			Expect(k8sClient.Create(ctx, &cl)).Should(Succeed())
		})

		// Controlplane resources
		for _, r := range cpResource {
			It(fmt.Sprintf("Check controlplane resource %s was created", reflect.TypeOf(r)), func() {
				checkResourceCreated(cpID, r)
			})
		}

		// Dataplane resources
		for _, r := range dpResource {
			It(fmt.Sprintf("Check dataplane resource %s was created", reflect.TypeOf(r)), func() {
				checkResourceCreated(dpID, r)
			})
		}

		// Ingress resource
		It("Check ingress resource not exist", func() {
			Expect(checkResourceNotExist(ingressID, &corev1.Service{})).Should(BeTrue())
		})
	})

	Context("Update ClusterLink deployment", func() {
		svc := corev1.Service{}
		It("Check ingress resource loadbalncer created", func() {
			checkResourceCreated(types.NamespacedName{Name: ClusterLinkName, Namespace: OperatorNameSpace}, &cl)
			cl.Spec.Ingress.Type = api.IngressTypeLB
			Expect(k8sClient.Update(ctx, &cl)).Should(Succeed())

			checkResourceCreated(ingressID, &svc)
			Expect(svc.Spec.Type == corev1.ServiceTypeLoadBalancer).Should(BeTrue())
		})

		It("Check ingress resource nodeport created", func() {
			Expect(k8sClient.Delete(ctx, &svc)).Should(Succeed())
			checkResourceCreated(types.NamespacedName{Name: ClusterLinkName, Namespace: OperatorNameSpace}, &cl)
			cl.Spec.Ingress.Type = api.IngressTypeNodePort
			Expect(k8sClient.Update(ctx, &cl)).Should(Succeed())
			checkResourceCreated(ingressID, &svc)
			Expect(svc.Spec.Type == corev1.ServiceTypeNodePort).Should(BeTrue())
		})
	})

	Context("Delete ClusterLink deployment", func() {
		It("Should delete ClusterLink deployment", func() {
			Expect(k8sClient.Delete(ctx, &cl)).Should(Succeed())
		})

		// Controlplane resources
		for _, r := range cpResource {
			It(fmt.Sprintf("Check controlplane resource %s was deleted", reflect.TypeOf(r)), func() {
				checkResourceDeleted(cpID, r)

			})
		}

		// Dataplane resources
		for _, r := range dpResource {
			It(fmt.Sprintf("Check dataplane resource %s was deleted", reflect.TypeOf(r)), func() {
				checkResourceDeleted(dpID, r)

			})
		}

		// Ingress resource
		It("Check ingress resource was deleted", func() {
			checkResourceDeleted(ingressID, &corev1.Service{})
		})
	})
})

func checkResourceDeleted(id types.NamespacedName, object client.Object) {
	Eventually(func() bool {
		err := k8sClient.Get(ctx, id, object)
		if err != nil {
			if apierrors.IsNotFound(err) {
				By(fmt.Sprintf("Resource was not found: %s", id.Name))
				return true
			}
			By(fmt.Sprintf("Error fetching resource: %v", err))
			return false
		}
		// Check if the resource has a deletion timestamp
		if !object.GetDeletionTimestamp().IsZero() {
			By(fmt.Sprintf("Deletion timestamp: %s", object.GetDeletionTimestamp().String()))
			return true
		}
		// If the resource exists and has no deletion timestamp, continue waiting
		return false
	}, Timeout, IntervalTime).Should(BeTrue())
}

func checkResourceCreated(id types.NamespacedName, object client.Object) {
	Eventually(func() bool {
		err := k8sClient.Get(context.Background(), id, object)
		return err == nil
	}, Timeout, IntervalTime).Should(BeTrue())
}

func checkResourceNotExist(id types.NamespacedName, object client.Object) bool {
	err := k8sClient.Get(ctx, id, object)
	if err != nil {
		if apierrors.IsNotFound(err) {
			By(fmt.Sprintf("Resource was not found: %s", id.Name))
			return true
		}
		By(fmt.Sprintf("Error fetching resource: %v", err))
		return false
	}
	By("Resource exist")
	return false

}

// 	Context("When updating CronJob Status", func() {
// 		It("Should increase CronJob Status.Active count when new Jobs are created", func() {
// 			By("By creating a new CronJob")
// 			ctx := context.Background()
// 			cronJob := &cronjobv1.CronJob{
// 				TypeMeta: metav1.TypeMeta{
// 					APIVersion: "batch.tutorial.kubebuilder.io/v1",
// 					Kind:       "CronJob",
// 				},
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      CronjobName,
// 					Namespace: CronjobNamespace,
// 				},
// 				Spec: cronjobv1.CronJobSpec{
// 					Schedule: "1 * * * *",
// 					JobTemplate: batchv1.JobTemplateSpec{
// 						Spec: batchv1.JobSpec{
// 							// For simplicity, we only fill out the required fields.
// 							Template: v1.PodTemplateSpec{
// 								Spec: v1.PodSpec{
// 									// For simplicity, we only fill out the required fields.
// 									Containers: []v1.Container{
// 										{
// 											Name:  "test-container",
// 											Image: "test-image",
// 										},
// 									},
// 									RestartPolicy: v1.RestartPolicyOnFailure,
// 								},
// 							},
// 						},
// 					},
// 				},
// 			}
// 			Expect(k8sClient.Create(ctx, cronJob)).Should(Succeed())

// 			/*
// 				After creating this CronJob, let's check that the CronJob's Spec fields match what we passed in.
// 				Note that, because the k8s apiserver may not have finished creating a CronJob after our `Create()` call from earlier, we will use Gomega’s Eventually() testing function instead of Expect() to give the apiserver an opportunity to finish creating our CronJob.

// 				`Eventually()` will repeatedly run the function provided as an argument every interval seconds until
// 				(a) the function’s output matches what’s expected in the subsequent `Should()` call, or
// 				(b) the number of attempts * interval period exceed the provided timeout value.

// 				In the examples below, timeout and interval are Go Duration values of our choosing.
// 			*/

// 			cronjobLookupKey := types.NamespacedName{Name: CronjobName, Namespace: CronjobNamespace}
// 			createdCronjob := &cronjobv1.CronJob{}

// 			// We'll need to retry getting this newly created CronJob, given that creation may not immediately happen.
// 			Eventually(func() bool {
// 				err := k8sClient.Get(ctx, cronjobLookupKey, createdCronjob)
// 				if err != nil {
// 					return false
// 				}
// 				return true
// 			}, timeout, interval).Should(BeTrue())
// 			// Let's make sure our Schedule string value was properly converted/handled.
// 			Expect(createdCronjob.Spec.Schedule).Should(Equal("1 * * * *"))
// 			/*
// 				Now that we've created a CronJob in our test cluster, the next step is to write a test that actually tests our CronJob controller’s behavior.
// 				Let’s test the CronJob controller’s logic responsible for updating CronJob.Status.Active with actively running jobs.
// 				We’ll verify that when a CronJob has a single active downstream Job, its CronJob.Status.Active field contains a reference to this Job.

// 				First, we should get the test CronJob we created earlier, and verify that it currently does not have any active jobs.
// 				We use Gomega's `Consistently()` check here to ensure that the active job count remains 0 over a duration of time.
// 			*/
// 			By("By checking the CronJob has zero active Jobs")
// 			Consistently(func() (int, error) {
// 				err := k8sClient.Get(ctx, cronjobLookupKey, createdCronjob)
// 				if err != nil {
// 					return -1, err
// 				}
// 				return len(createdCronjob.Status.Active), nil
// 			}, duration, interval).Should(Equal(0))
// 			/*
// 				Next, we actually create a stubbed Job that will belong to our CronJob, as well as its downstream template specs.
// 				We set the Job's status's "Active" count to 2 to simulate the Job running two pods, which means the Job is actively running.

// 				We then take the stubbed Job and set its owner reference to point to our test CronJob.
// 				This ensures that the test Job belongs to, and is tracked by, our test CronJob.
// 				Once that’s done, we create our new Job instance.
// 			*/
// 			By("By creating a new Job")
// 			testJob := &batchv1.Job{
// 				ObjectMeta: metav1.ObjectMeta{
// 					Name:      JobName,
// 					Namespace: CronjobNamespace,
// 				},
// 				Spec: batchv1.JobSpec{
// 					Template: v1.PodTemplateSpec{
// 						Spec: v1.PodSpec{
// 							// For simplicity, we only fill out the required fields.
// 							Containers: []v1.Container{
// 								{
// 									Name:  "test-container",
// 									Image: "test-image",
// 								},
// 							},
// 							RestartPolicy: v1.RestartPolicyOnFailure,
// 						},
// 					},
// 				},
// 				Status: batchv1.JobStatus{
// 					Active: 2,
// 				},
// 			}

// 			// Note that your CronJob’s GroupVersionKind is required to set up this owner reference.
// 			kind := reflect.TypeOf(cronjobv1.CronJob{}).Name()
// 			gvk := cronjobv1.GroupVersion.WithKind(kind)

// 			controllerRef := metav1.NewControllerRef(createdCronjob, gvk)
// 			testJob.SetOwnerReferences([]metav1.OwnerReference{*controllerRef})
// 			Expect(k8sClient.Create(ctx, testJob)).Should(Succeed())
// 			/*
// 				Adding this Job to our test CronJob should trigger our controller’s reconciler logic.
// 				After that, we can write a test that evaluates whether our controller eventually updates our CronJob’s Status field as expected!
// 			*/
// 			By("By checking that the CronJob has one active Job")
// 			Eventually(func() ([]string, error) {
// 				err := k8sClient.Get(ctx, cronjobLookupKey, createdCronjob)
// 				if err != nil {
// 					return nil, err
// 				}

// 				names := []string{}
// 				for _, job := range createdCronjob.Status.Active {
// 					names = append(names, job.Name)
// 				}
// 				return names, nil
// 			}, timeout, interval).Should(ConsistOf(JobName), "should list our active job %s in the active jobs list in status", JobName)
// 		})
// 	})

// })

// /*
// 	After writing all this code, you can run `go test ./...` in your `controllers/` directory again to run your new test!
// */
