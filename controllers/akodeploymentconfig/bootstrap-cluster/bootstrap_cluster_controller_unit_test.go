// Copyright (c) 2020 VMware, Inc. All Rights Reserved.
// SPDX-License-Identifier: Apache-2.0

package bootstrap_cluster_test

import (
	bootstrap_cluster "gitlab.eng.vmware.com/core-build/ako-operator/controllers/akodeploymentconfig/bootstrap-cluster"
	ako_operator "gitlab.eng.vmware.com/core-build/ako-operator/pkg/ako-operator"
	"os"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"
	akoov1alpha1 "gitlab.eng.vmware.com/core-build/ako-operator/api/v1alpha1"
)

func unitTestAKODeploymentYaml() {
	Context("PopulateValues", func() {
		var (
			akoDeploymentConfig *akoov1alpha1.AKODeploymentConfig
		)

		JustBeforeEach(func() {
			os.Setenv(ako_operator.ManagementClusterName, "test")
		})

		When("a valid AKODeploymentYaml is provided", func() {
			BeforeEach(func() {
				akoDeploymentConfig = &akoov1alpha1.AKODeploymentConfig{
					Spec: akoov1alpha1.AKODeploymentConfigSpec{
						CloudName:          "test-cloud",
						Controller:         "10.23.122.1",
						ServiceEngineGroup: "Default-SEG",
						DataNetwork: akoov1alpha1.DataNetwork{
							Name: "test-akdc",
							CIDR: "10.0.0.0/24",
						},
						ExtraConfigs: akoov1alpha1.ExtraConfigs{
							Image: akoov1alpha1.AKOImageConfig{
								Repository: "test/image",
								PullPolicy: "IfNotPresent",
								Version:    "1.3.1",
							},
							Rbac: akoov1alpha1.AKORbacConfig{
								PspEnabled:          true,
								PspPolicyAPIVersion: "test/1.2",
							},
							Log: akoov1alpha1.AKOLogConfig{
								PersistentVolumeClaim: "true",
								MountPath:             "/var/log",
								LogFile:               "test-avi.log",
							},
							IngressConfigs: akoov1alpha1.AKOIngressConfig{
								DisableIngressClass:      true,
								DefaultIngressController: true,
								ShardVSSize:              "MEDIUM",
								ServiceType:              "NodePort",
								NodeNetworkList: []akoov1alpha1.NodeNetwork{
									{
										NetworkName: "test-node-network-1",
										Cidrs:       []string{"10.0.0.0/24", "192.168.0.0/24"},
									},
								},
							},
							DisableStaticRouteSync: true,
						},
					},
				}
			})

			It("should populate correct values in crs yaml", func() {
				_, err := bootstrap_cluster.ConvertToAKODeploymentYaml(akoDeploymentConfig)
				Expect(err).ShouldNot(HaveOccurred())
			})

			It("should throw error if template not match", func() {
				akoDeploymentConfig.Spec.DataNetwork.CIDR = "test"
				_, err := bootstrap_cluster.ConvertToAKODeploymentYaml(akoDeploymentConfig)
				Expect(err).Should(HaveOccurred())
				akoDeploymentConfig.Spec.DataNetwork.CIDR = "10.0.0.0/24"
			})
		})
	})
}
