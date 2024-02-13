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

package k8s

import (
	"fmt"
	"time"

	"github.com/stretchr/testify/require"

	clusterlink "github.com/clusterlink-net/clusterlink/pkg/apis/clusterlink.net/v1alpha1"
	"github.com/clusterlink-net/clusterlink/pkg/operator/controller"
	"github.com/clusterlink-net/clusterlink/tests/e2e/k8s/services/httpecho"
	"github.com/clusterlink-net/clusterlink/tests/e2e/k8s/util"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

func (s *TestSuite) TestOperatorConnectivity() {
	s.RunOnAllDataplaneTypes(func(cfg *util.PeerConfig) {
		cfg.DeployWithOperator = true
		cl, err := s.fabric.DeployClusterlinks(2, cfg)
		require.Nil(s.T(), err)

		require.Nil(s.T(), cl[0].CreateExport("echo", &httpEchoService))
		require.Nil(s.T(), cl[0].CreatePolicy(util.PolicyAllowAll))
		require.Nil(s.T(), cl[1].CreatePeer(cl[0]))

		importedService := &util.Service{
			Name: "echo1",
			Port: 80,
		}
		require.Nil(s.T(), cl[1].CreateImport("echo", importedService))

		require.Nil(s.T(), cl[1].CreateBinding("echo", cl[0]))
		require.Nil(s.T(), cl[1].CreatePolicy(util.PolicyAllowAll))

		data, err := cl[1].AccessService(httpecho.GetEchoValue, importedService, true, nil)
		require.Nil(s.T(), err)
		require.Equal(s.T(), cl[0].Name(), data)
		instance := &clusterlink.Instance{}
		s.fabric.GetResource(0, "cl-instance", controller.OperatorNameSpace, instance)
		fmt.Println("instance", instance)
		require.Equal(s.T(), metav1.ConditionTrue, instance.Status.Controlplane.Conditions[string(clusterlink.DeploymentReady)].Status)
		require.Equal(s.T(), metav1.ConditionTrue, instance.Status.Dataplane.Conditions[string(clusterlink.DeploymentReady)].Status)
		require.Equal(s.T(), metav1.ConditionTrue, instance.Status.Ingress.Conditions[string(clusterlink.ServiceReady)].Status)

		instance2 := &clusterlink.Instance{
			ObjectMeta: metav1.ObjectMeta{
				Name:      "instance2",
				Namespace: controller.OperatorNameSpace,
			},
			Spec: clusterlink.InstanceSpec{Namespace: s.fabric.GetNamespace()},
		}
		err = s.fabric.CreateResource(0, instance2)
		if err != nil {
			fmt.Println("error", err)
		}
		s.fabric.GetResource(0, "instance2", controller.OperatorNameSpace, instance2)
		time.Sleep(20 * time.Second)
		fmt.Println("instance2", instance2)
		fmt.Println("instance2", instance2.Status.Controlplane.Conditions[string(clusterlink.DeploymentReady)])
		fmt.Println("instance2", instance2.Status.Dataplane.Conditions[string(clusterlink.DeploymentReady)])

		require.Equal(s.T(), instance2.Status.Controlplane.Conditions[string(clusterlink.DeploymentReady)].Status, metav1.ConditionFalse)
		require.Equal(s.T(), instance2.Status.Dataplane.Conditions[string(clusterlink.DeploymentReady)].Status, metav1.ConditionFalse)
	})
}
