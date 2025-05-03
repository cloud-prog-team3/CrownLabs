// Copyright 2020-2025 Politecnico di Torino
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

// Package instautoctrl contains the controller for Instance Inactive Termination
package instinactivectrl

import (
	"context"
	"fmt"
	"time"

	kerrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/tools/record"
	"k8s.io/utils/trace"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller"

	clv1alpha2 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha2"
	"github.com/netgroup-polito/CrownLabs/operators/pkg/utils"
	"sigs.k8s.io/controller-runtime/pkg/event"
	"sigs.k8s.io/controller-runtime/pkg/predicate"
)

// InstanceInactiveTerminationReconciler watches for instances to be terminated.
type InstanceInactiveTerminationReconciler struct {
	client.Client
	EventsRecorder              record.EventRecorder
	Scheme                      *runtime.Scheme
	NamespaceWhitelist          metav1.LabelSelector
	StatusCheckRequestTimeout   time.Duration
	InstanceStatusCheckInterval time.Duration
	// This function, if configured, is deferred at the beginning of the Reconcile.
	// Specifically, it is meant to be set to GinkgoRecover during the tests,
	// in order to lead to a controlled failure in case the Reconcile panics.
	ReconcileDeferHook func()
}

// SetupWithManager registers a new controller for InstanceTerminationReconciler resources.
func (r *InstanceInactiveTerminationReconciler) SetupWithManager(mgr ctrl.Manager, concurrency int) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clv1alpha2.Instance{}).
		Named("instance-inactive-termination").
		WithOptions(controller.Options{
			MaxConcurrentReconciles: concurrency,
		}).
		WithEventFilter(predicate.Funcs{
			UpdateFunc: func(e event.UpdateEvent) bool {
				return false
			},
		}).
		WithLogConstructor(utils.LogConstructor(mgr.GetLogger(), "InstanceInactiveTermination")).
		Complete(r)
}

// Reconcile reconciles the status of the InstanceSnapshot resource.
func (r *InstanceInactiveTerminationReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	if r.ReconcileDeferHook != nil {
		defer r.ReconcileDeferHook()
	}

	log := ctrl.LoggerFrom(ctx, "instance", req.NamespacedName)
	dbgLog := log.V(utils.LogDebugLevel)
	tracer := trace.New("reconcile", trace.Field{Key: "instance", Value: req.NamespacedName})
	ctx = ctrl.LoggerInto(trace.ContextWithTrace(ctx, tracer), log)

	// if dbgLog.Enabled() {
	// 	defer tracer.Log()
	// } else {
	// 	defer tracer.LogIfLong(r.StatusCheckRequestTimeout / 2)
	// }

	// Get the instance object.
	var instance clv1alpha2.Instance
	if err := r.Get(ctx, req.NamespacedName, &instance); err != nil {
		if !kerrors.IsNotFound(err) {
			log.Error(err, "failed retrieving instance")
		}
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}

	tracer.Step("instance retrieved")

	terminate, err := r.CheckInstanceTermination(ctx, &instance)
	if err != nil {
		log.Error(err, "failed checking instance termination")
		return ctrl.Result{}, err
	}
	if terminate {
		// retrieve the user owner of the instance
		user, err := r.GetTenantFromInstance(ctx, &instance)
		if err != nil {
			log.Error(err, "failed retrieving user from instance")
			return ctrl.Result{}, err
		}

		// send notification to the user
		if instance.Status.TerminationAlerts < 3 {
			r.SendNotification(ctx, &instance, user.Spec.Email)

		} else if instance.Status.TerminationAlerts >= 3 && instance.Spec.Running == true {
			r.TerminateInstance(ctx, &instance)
		}
	} else {
		log.Info("instance is not yet to be terminated", "instance", instance.Name)
	}

	dbgLog.Info("requeueing instance")
	return ctrl.Result{RequeueAfter: 2 * time.Minute}, nil
}

// CheckInstanceTermination checks if the Instance has to be terminated.
func (r *InstanceInactiveTerminationReconciler) CheckInstanceTermination(ctx context.Context, instance *clv1alpha2.Instance) (bool, error) {

	age := time.Since(instance.ObjectMeta.GetCreationTimestamp().Time)
	if age > 1*time.Minute {
		return true, nil
	}

	return false, nil
}

// TerminateInstance terminates the Instance.
func (r *InstanceInactiveTerminationReconciler) TerminateInstance(ctx context.Context, instance *clv1alpha2.Instance) error {
	log := ctrl.LoggerFrom(ctx).WithName("termination")
	log.Info("terminating instance")

	instance.Spec.Running = false

	return r.Update(ctx, instance)
}

// SendNotification sends an email to the user to notify that the instance will be terminated/stopped if they do not use it anymore.
func (r *InstanceInactiveTerminationReconciler) SendNotification(ctx context.Context, instance *clv1alpha2.Instance, userEmail string) error {
	// TODO: implement the email notification
	log := ctrl.LoggerFrom(ctx).WithName("notification-email-instance")
	log.Info("sending email notification to user", "instance", instance.Name, "email", userEmail)

	// increment the number of termination alerts
	instance.Status.TerminationAlerts++
	if err := r.Status().Update(ctx, instance); err != nil {
		log.Error(err, "failed updating instance status")
	}

	return nil
}

func (r *InstanceInactiveTerminationReconciler) GetTenantFromInstance(ctx context.Context, instance *clv1alpha2.Instance) (clv1alpha2.Tenant, error) {
	log := ctrl.LoggerFrom(ctx).WithName("get-user-from-instance")
	log.Info("getting user from instance", "instance", instance.Name)

	tenant := &clv1alpha2.Tenant{}
	if err := r.Client.Get(ctx, client.ObjectKey{
		Name:      instance.Spec.Tenant.Name,
		Namespace: instance.Namespace,
	}, tenant); err != nil {
		if kerrors.IsNotFound(err) {
			log.Error(err, "user not found")
			return clv1alpha2.Tenant{}, fmt.Errorf("user %s not found", instance.Spec.Tenant.Name)
		}
		log.Error(err, "failed retrieving user")
		return clv1alpha2.Tenant{}, err
	}
	return *tenant, nil
}
