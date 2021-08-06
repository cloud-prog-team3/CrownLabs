// Copyright 2020-2021 Politecnico di Torino
//
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

package instance_controller_test

import (
	"time"

	. "github.com/onsi/ginkgo"
	. "github.com/onsi/ginkgo/extensions/table"
	. "github.com/onsi/gomega"
	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	virtv1 "kubevirt.io/client-go/api/v1"

	clv1alpha2 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha2"
	instance_controller "github.com/netgroup-polito/CrownLabs/operators/pkg/instance-controller"
)

var _ = Describe("Status Inspection", func() {

	Describe("The statusinspection.RetrievePhaseFromVM function", func() {
		var reconciler instance_controller.InstanceReconciler

		ForgeVM := func(status virtv1.VirtualMachinePrintableStatus) *virtv1.VirtualMachine {
			return &virtv1.VirtualMachine{Status: virtv1.VirtualMachineStatus{PrintableStatus: status, Ready: false}}
		}

		ForgeReadyVM := func() *virtv1.VirtualMachine {
			return &virtv1.VirtualMachine{Status: virtv1.VirtualMachineStatus{
				PrintableStatus: virtv1.VirtualMachineStatusRunning,
				Ready:           true,
			}}
		}

		BeforeEach(func() {
			reconciler = instance_controller.InstanceReconciler{}
		})

		DescribeTable("Correctly returns the expected instance phase",
			func(vm *virtv1.VirtualMachine, expected clv1alpha2.EnvironmentPhase) {
				Expect(reconciler.RetrievePhaseFromVM(vm)).To(Equal(expected))
			},
			Entry("When the VM is starting", ForgeVM(virtv1.VirtualMachineStatusStarting), clv1alpha2.EnvironmentPhaseStarting),
			Entry("When the VM is provisioning", ForgeVM(virtv1.VirtualMachineStatusProvisioning), clv1alpha2.EnvironmentPhaseImporting),
			Entry("When the VM is stopping", ForgeVM(virtv1.VirtualMachineStatusStopping), clv1alpha2.EnvironmentPhaseStopping),
			Entry("When the VM is terminating", ForgeVM(virtv1.VirtualMachineStatusTerminating), clv1alpha2.EnvironmentPhaseStopping),
			Entry("When the VM is off", ForgeVM(virtv1.VirtualMachineStatusStopped), clv1alpha2.EnvironmentPhaseOff),
			Entry("When the VM is running", ForgeVM(virtv1.VirtualMachineStatusRunning), clv1alpha2.EnvironmentPhaseRunning),
			Entry("When the VM is ready", ForgeReadyVM(), clv1alpha2.EnvironmentPhaseReady),
			Entry("When the VM status is unknown", ForgeVM(virtv1.VirtualMachineStatusUnknown), clv1alpha2.EnvironmentPhaseUnset),
		)
	})

	Describe("The statusinspection.RetrievePhaseFromVMI function", func() {
		var reconciler instance_controller.InstanceReconciler

		ForgeVMI := func(phase virtv1.VirtualMachineInstancePhase) *virtv1.VirtualMachineInstance {
			return &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
				Phase: phase,
				Conditions: []virtv1.VirtualMachineInstanceCondition{
					{Type: virtv1.VirtualMachineInstanceReady, Status: corev1.ConditionFalse},
					{Type: virtv1.VirtualMachineInstanceIsMigratable, Status: corev1.ConditionTrue},
					{Type: virtv1.VirtualMachineInstancePaused, Status: corev1.ConditionFalse},
				},
			}}
		}

		ForgeReadyVMI := func() *virtv1.VirtualMachineInstance {
			return &virtv1.VirtualMachineInstance{Status: virtv1.VirtualMachineInstanceStatus{
				Phase: virtv1.Running,
				Conditions: []virtv1.VirtualMachineInstanceCondition{
					{Type: virtv1.VirtualMachineInstanceReady, Status: corev1.ConditionTrue},
					{Type: virtv1.VirtualMachineInstanceIsMigratable, Status: corev1.ConditionTrue},
					{Type: virtv1.VirtualMachineInstancePaused, Status: corev1.ConditionFalse},
				},
			}}
		}

		ForgeStoppingVMI := func() *virtv1.VirtualMachineInstance {
			timestamp := metav1.NewTime(time.Now())
			return &virtv1.VirtualMachineInstance{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &timestamp}}
		}

		BeforeEach(func() {
			reconciler = instance_controller.InstanceReconciler{}
		})

		DescribeTable("Correctly returns the expected instance phase",
			func(vmi *virtv1.VirtualMachineInstance, expected clv1alpha2.EnvironmentPhase) {
				Expect(reconciler.RetrievePhaseFromVMI(vmi)).To(Equal(expected))
			},
			Entry("When the VMI status is unset", ForgeVMI(virtv1.VmPhaseUnset), clv1alpha2.EnvironmentPhaseUnset),
			Entry("When the VMI is pending", ForgeVMI(virtv1.Pending), clv1alpha2.EnvironmentPhaseStarting),
			Entry("When the VMI is scheduling", ForgeVMI(virtv1.Scheduling), clv1alpha2.EnvironmentPhaseStarting),
			Entry("When the VMI is scheduled", ForgeVMI(virtv1.Scheduled), clv1alpha2.EnvironmentPhaseStarting),
			Entry("When the VMI is running", ForgeVMI(virtv1.Running), clv1alpha2.EnvironmentPhaseRunning),
			Entry("When the VMI is ready", ForgeReadyVMI(), clv1alpha2.EnvironmentPhaseReady),
			Entry("When the VMI status is unknown", ForgeVMI(virtv1.Unknown), clv1alpha2.EnvironmentPhaseFailed),
			Entry("When the VMI status is failed", ForgeVMI(virtv1.Failed), clv1alpha2.EnvironmentPhaseFailed),
			Entry("When the VMI status is succeeded", ForgeVMI(virtv1.Succeeded), clv1alpha2.EnvironmentPhaseFailed),
			Entry("When the VMI is being deleted", ForgeStoppingVMI(), clv1alpha2.EnvironmentPhaseStopping),
		)
	})

	Describe("The statusinspection.RetrievePhaseFromDeployment function", func() {
		var reconciler instance_controller.InstanceReconciler

		ForgeDeployment := func(desired, ready int32) *appsv1.Deployment {
			return &appsv1.Deployment{
				Spec:   appsv1.DeploymentSpec{Replicas: &desired},
				Status: appsv1.DeploymentStatus{ReadyReplicas: ready},
			}
		}

		ForgeStoppingDeployment := func() *appsv1.Deployment {
			timestamp := metav1.NewTime(time.Now())
			return &appsv1.Deployment{ObjectMeta: metav1.ObjectMeta{DeletionTimestamp: &timestamp}}
		}

		BeforeEach(func() {
			reconciler = instance_controller.InstanceReconciler{}
		})

		DescribeTable("Correctly returns the expected instance phase",
			func(deployment *appsv1.Deployment, expected clv1alpha2.EnvironmentPhase) {
				Expect(reconciler.RetrievePhaseFromDeployment(deployment)).To(Equal(expected))
			},
			Entry("When the deployment has no replicas", ForgeDeployment(0, 0), clv1alpha2.EnvironmentPhaseOff),
			Entry("When the deployment replicas are not ready", ForgeDeployment(1, 0), clv1alpha2.EnvironmentPhaseStarting),
			Entry("When the deployment replicas are ready", ForgeDeployment(1, 1), clv1alpha2.EnvironmentPhaseReady),
			Entry("When the deployment is being deleted", ForgeStoppingDeployment(), clv1alpha2.EnvironmentPhaseStopping),
		)
	})
})
