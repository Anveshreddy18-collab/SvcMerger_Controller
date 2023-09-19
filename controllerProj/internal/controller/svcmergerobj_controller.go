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
	"time"

	// metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/apimachinery/pkg/util/intstr"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/controller/controllerutil"
	"sigs.k8s.io/controller-runtime/pkg/log"

	// "sigs.k8s.io/controller-runtime/pkg/reconcile"

	newprojv1 "controllerProj/api/v1"
	"fmt"
)

// SvcMergerObjReconciler reconciles a SvcMergerObj object
type SvcMergerObjReconciler struct {
	client.Client
	Scheme *runtime.Scheme
}

var all_maps_initialized bool = false

// stores the names of merged services
var merged_service_exists map[string]bool

// map to store the service names which are currently under merger
var cur_mrgd_svcs_map map[string]map[string]int

// List of pods that are currently under merger.
var merged_pods map[string][]string

// Map to store svc name and port number
var svc_port_map map[string]int32

type string_pair struct {
	pod     string
	service string
}

// This function will take in a list of services and return a list of pods that are associated with those services
func (r *SvcMergerObjReconciler) getPodNames(ctx context.Context, req ctrl.Request, services []string) ([]string_pair, error) {

	l := log.FromContext(ctx)
	l.Info("Entered getPodNames function")

	// create a pods array of pair of {string, string} to store the pod name and service name
	var pods []string_pair

	for _, svc := range services {

		service := &corev1.Service{}
		err := r.Get(ctx, types.NamespacedName{
			Name:      svc,
			Namespace: req.Namespace,
		}, service)

		if err != nil {
			l.Error(err, "not able to fetch service")
			return nil, err
		}

		pod_list := &corev1.PodList{}
		selector_labels_map := service.Spec.Selector
		err = r.Client.List(ctx, pod_list, client.InNamespace(req.Namespace), client.MatchingLabels(selector_labels_map))
		if err != nil {
			l.Error(err, "not able to fetch pods")
			return nil, err
		}

		for _, pod := range pod_list.Items {
			pods = append(pods, string_pair{pod.Name, svc})
		}
	}
	return pods, nil
}

// This function will take in a pod object and return the deployment owner reference
func (r *SvcMergerObjReconciler) getDeploymentName(ctx context.Context, req ctrl.Request, pod_obj *corev1.Pod) (string, error) {

	l := log.FromContext(ctx)
	owner_ref := pod_obj.OwnerReferences

	// this owner_ref is a replica set object, we need to get the deployment object from it
	for _, owner := range owner_ref {
		if owner.Kind == "ReplicaSet" {
			replica_set_obj := &appsv1.ReplicaSet{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      owner.Name,
				Namespace: req.Namespace,
			}, replica_set_obj)
			if err != nil {
				l.Error(err, "not able to fetch replica set")
				return "", err

			}
			owner_ref2 := replica_set_obj.OwnerReferences
			for _, owner2 := range owner_ref2 {
				if owner2.Kind == "Deployment" {
					return owner2.Name, nil
				}
			}

		}
	}
	return "", nil
}

//+kubebuilder:rbac:groups=newproj.controller.proj,resources=svcmergerobjs,verbs=get;list;watch;create;update;patch;delete
//+kubebuilder:rbac:groups=newproj.controller.proj,resources=svcmergerobjs/status,verbs=get;update;patch
//+kubebuilder:rbac:groups=newproj.controller.proj,resources=svcmergerobjs/finalizers,verbs=update

// Reconcile is part of the main kubernetes reconciliation loop which aims to
// move the current state of the cluster closer to the desired state.
// TODO(user): Modify the Reconcile function to compare the state specified by
// the SvcMergerObj object against the actual cluster state, and then
// perform operations to make the cluster state reflect the state specified by
// the user.
//
// For more details, check Reconcile and its Result here:
// - https://pkg.go.dev/sigs.k8s.io/controller-runtime@v0.15.0/pkg/reconcile
func (r *SvcMergerObjReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	l := log.FromContext(ctx)
	time.Sleep(3 * time.Second)
	l.Info(fmt.Sprintf("Entered Reconciliation function >>>> %v", req.Name))
	var name string
	var delete_event bool
	// Get the name of the Custom Resource
	svcMergerObj := &newprojv1.SvcMergerObj{}
	err := r.Get(ctx, req.NamespacedName, svcMergerObj)
	if err != nil {
		return ctrl.Result{}, client.IgnoreNotFound(err)
	}
	// fmt.Println("################", svcMergerObj.ObjectMeta.Name)
	// if svcMergerObj.ObjectMeta.Name == "" {
	// 	return ctrl.Result{}, nil
	// }
	if svcMergerObj.Kind == "SvcMergerObj" {
		finalizer := "finalizer.newproj.controller.proj"
		if svcMergerObj.DeletionTimestamp.IsZero() {

			name = svcMergerObj.ObjectMeta.Name
			delete_event = false
			if !controllerutil.ContainsFinalizer(svcMergerObj, finalizer) {
				controllerutil.AddFinalizer(svcMergerObj, finalizer)
				if err := r.Update(ctx, svcMergerObj); err != nil {
					l.Info("error in adding finalizer")
					return ctrl.Result{}, err
				}
			}
		} else {
			if controllerutil.ContainsFinalizer(svcMergerObj, finalizer) {
				l.Info("deleting the finalizer")
				name = svcMergerObj.ObjectMeta.Name
				delete_event = true
				controllerutil.RemoveFinalizer(svcMergerObj, finalizer)
				if err := r.Update(ctx, svcMergerObj); err != nil {
					l.Info("error in removing finalizer")
					return ctrl.Result{}, err
				}
			}
		}
	}

	if !all_maps_initialized {

		merged_service_exists = make(map[string]bool)
		cur_mrgd_svcs_map = make(map[string]map[string]int)
		svc_port_map = make(map[string]int32)
		merged_pods = make(map[string][]string)

		all_maps_initialized = true
	}

	// Check if the merged service "name" exists or not. If not, then create it.
	if _, exists := merged_service_exists[name]; !exists {

		fmt.Println("######################################### CREATION STARTED ###############################################")
		l.Info("Merging services ............")
		var services []string // get the services from the spec

		services = svcMergerObj.Spec.Services
		cur_mrgd_svcs_map[name] = make(map[string]int)

		// Fill the cur_mrgd_svcs_map with service name and labels
		for _, svc := range services {
			service := &corev1.Service{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      svc,
				Namespace: req.Namespace,
			}, service)
			if err != nil {
				l.Error(err, "not able to fetch service")
				return ctrl.Result{}, err
			}
			// fmt.Println("Ig it's here###########")
			cur_mrgd_svcs_map[name][svc] = 1 // just to inform that it's there!
			svc_port_map[svc] = service.Spec.Ports[0].Port
		}

		var pods []string_pair

		pods, err := r.getPodNames(ctx, req, services)
		if err != nil {
			l.Error(err, "not able to get pods -- first time")
			return ctrl.Result{}, err
		}

		fmt.Println("all pods before merging")
		for _, pod := range pods {
			l.Info(pod.pod)
		}

		deployment_map := make(map[string]bool)

		// Now that we have all the pods, we need to loop over them and get the owner references of each pod and add a new merge label to the deployment if it doesn't exist already
		for _, pod := range pods {
			pod_obj := &corev1.Pod{}
			err = r.Get(ctx, types.NamespacedName{
				Name:      pod.pod,
				Namespace: req.Namespace,
			}, pod_obj)
			if err != nil {
				// l.Error(err, "not able to fetch pod")
				// return ctrl.Result{}, err
				l.Info("pod not found, but that's okay, as we don't have to update the deployment again")
				continue
			}

			// I want to change the label selector of the deployment to include the new label, for that I need to get the owner reference of the pod
			// and then get the deployment object to add the label to it and update it
			// owner_ref := pod_obj.OwnerReferences

			//maintain a map to see if this deployment has already been fetched. If yes, then we don't need to do again

			deployment_name, err := r.getDeploymentName(ctx, req, pod_obj)
			if deployment_name == "" {
				l.Error(err, "Deployment name is empty")
				return ctrl.Result{}, err
			}
			if err != nil {
				l.Error(err, "not able to get deployment name")
				return ctrl.Result{}, err
			}

			if deployment_map[deployment_name] == false {

				deployment_obj := &appsv1.Deployment{}
				err = r.Get(ctx, types.NamespacedName{
					Name:      deployment_name,
					Namespace: req.Namespace,
				}, deployment_obj)
				if err != nil {
					l.Error(err, "not able to fetch deployment")
					return ctrl.Result{}, err
				}
				fmt.Println(">>>>>>>>", deployment_name)

				// add  'merge' label & name = pod.service to the pod template of deployment object if it doesn't exist already
				pod_template_labels := deployment_obj.Spec.Template.Labels
				if pod_template_labels["merge"] != name {
					pod_template_labels["merge"] = name
					pod_template_labels["name"] = pod.service
					deployment_obj.Spec.Template.SetLabels(pod_template_labels)
					err = r.Update(ctx, deployment_obj)
					if err != nil {
						l.Error(err, "not able to update deployment with a label")
						return ctrl.Result{}, err
					}
				}

				deployment_map[deployment_name] = true
			}

		}
		// update the pods array with names of new pods that are created because of the restart of deployment
		// sleep for 5 seconds to give time for the pods to restart
		time.Sleep(20 * time.Second)
		pods = nil
		pods, err = r.getPodNames(ctx, req, services)
		if err != nil {
			l.Error(err, "not able to get pods -- second time")
			return ctrl.Result{}, err
		}

		fmt.Println("all pods after merging")
		for _, pod := range pods {
			l.Info(pod.pod)
		}

		// this is the final list of pods that are created after the merge operation
		merged_pods[name] = []string{}
		for _, pod := range pods {
			merged_pods[name] = append(merged_pods[name], pod.pod)
		}

		// Now we need to create a new service with selector label as merge=true to add the pods from the array 'pods'
		merged_svc := &corev1.Service{}
		merged_svc.Name = name
		merged_svc.Namespace = req.Namespace
		merged_svc.Spec.Selector = map[string]string{
			"merge": name,
		}
		port_no := 89 + int32(len(merged_service_exists))
		merged_svc.Spec.Ports = []corev1.ServicePort{
			{
				Name:       "merged-service-port",
				Port:       port_no,
				Protocol:   corev1.ProtocolTCP,
				TargetPort: intstr.FromInt(8080),
			},
		}
		finalizer := "finalizer.newproj.controller.proj/" + name
		merged_svc.Finalizers = append(merged_svc.Finalizers, finalizer)
		err = r.Create(ctx, merged_svc)
		if err != nil {
			l.Error(err, "not able to create new merge service")
			return ctrl.Result{}, err
		}
		// Deleting all the services from the services array
		for _, svc := range services {
			service := &corev1.Service{}
			err = r.Get(ctx, types.NamespacedName{
				Name:      svc,
				Namespace: req.Namespace,
			}, service)
			if err != nil {
				l.Error(err, "not able to fetch service", "service", service)
				return ctrl.Result{}, err
			}
			err = r.Delete(ctx, service)
			if err != nil {
				l.Error(err, "could not delete service", "service", service)
				return ctrl.Result{}, err
			}
		}
		// Add the merged service to the merged_service_exists map
		merged_service_exists[name] = true
		return ctrl.Result{}, nil
	} else {

		// This gets triggered when the crd is deleted or updated.
		var services []string // stores the services fetched from the spec of svcmergerobj

		if delete_event == false {

			fmt.Println("####################################### UPDATION STARTED ###############################################")
			l.Info("Svcmergerobj is not deleted. So Reconciler Update.")
			services = svcMergerObj.Spec.Services
			new_service_map := make(map[string]int)
			for _, svc := range services {
				new_service_map[svc] = 1
			}

			// Maintain two lists to_delete, to_add
			var to_delete []string
			var to_add []string

			var all_services []string
			for svc := range cur_mrgd_svcs_map[name] {
				all_services = append(all_services, svc)
			}
			for svc := range new_service_map {
				all_services = append(all_services, svc)
			}

			for _, svc := range all_services {
				_, is_old := cur_mrgd_svcs_map[name][svc]
				_, is_new := new_service_map[svc]

				if is_old && is_new {
					continue
				} else if is_old {
					to_delete = append(to_delete, svc)
				} else {
					to_add = append(to_add, svc)
				}
			}

			fmt.Println("size of to_add is ", len(to_add))
			fmt.Println("size of to_delete is", len(to_delete))

			// Delete(Liberate) the pods associated with the services in to_delete
			for _, svc := range to_delete {

				// create a service with the name and labels as temp map
				var temp = make(map[string]string)
				temp["name"] = svc
				new_svc := &corev1.Service{}
				new_svc.Name = svc
				new_svc.Namespace = req.Namespace
				new_svc.Spec.Selector = temp
				new_svc.Spec.Ports = []corev1.ServicePort{
					{
						Name:       "merged-service-port",
						Port:       svc_port_map[svc], // port number retrieved from the service port map
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(8080),
					},
				}
				err := r.Create(ctx, new_svc)
				if err != nil {
					l.Error(err, "not able to create new service")
					return ctrl.Result{}, err
				}
				// Service is created. But currently there are no pods associated with it. If we remove "merge"="true" label from deployment,
				// then the pods will be freed. So we need to get the deployment name from the pod name and then remove the label from the deployment
				// But first we need to get the pod list.

				// Add merge=true label to the temp map. Used to identify the pods that were under this service before merging
				temp["merge"] = name
				// Now we need to get all the pods in the cluster with the labels in temp
				pod_list := &corev1.PodList{}
				err = r.Client.List(ctx, pod_list, client.InNamespace(req.Namespace), client.MatchingLabels(temp))
				if err != nil {
					l.Error(err, "Unable to get pod list from matching labels")
					return ctrl.Result{}, err
				}
				// Now we need to get the deployment name from the pod name and then remove the label from the deployment
				deployment_map := make(map[string]bool)
				for _, pod := range pod_list.Items {
					deployment_name, err := r.getDeploymentName(ctx, req, &pod)
					if err != nil {
						l.Error(err, "not able to get deployment name")
						return ctrl.Result{}, err
					}
					if deployment_map[deployment_name] == false {

						deployment_obj := &appsv1.Deployment{}
						err = r.Get(ctx, types.NamespacedName{
							Name:      deployment_name,
							Namespace: req.Namespace,
						}, deployment_obj)
						if err != nil {
							l.Error(err, "not able to fetch deployment")
							return ctrl.Result{}, err
						}
						pod_template_labels := deployment_obj.Spec.Template.Labels
						delete(pod_template_labels, "merge")
						deployment_obj.Spec.Template.SetLabels(pod_template_labels)
						err = r.Update(ctx, deployment_obj)
						if err != nil {
							l.Error(err, "not able to delete label from deployment")
							return ctrl.Result{}, err
						}

						deployment_map[deployment_name] = true
					}
				}

				// Delete the svc from cur_mrgd_svcs_map and svc_port_map
				delete(cur_mrgd_svcs_map[name], svc)
				delete(svc_port_map, svc)

			}

			// Add the pods associated with the services in to_add to the merged service by adding the labels to the deployment
			for _, svc := range to_add {

				svc_obj := &corev1.Service{}
				err := r.Get(ctx, types.NamespacedName{
					Name:      svc,
					Namespace: req.Namespace,
				}, svc_obj)
				if err != nil {
					l.Error(err, "unable to fetch service")
					return ctrl.Result{}, err
				}
				// Add the service name and labels to the cur_mrgd_svcs_map
				cur_mrgd_svcs_map[name][svc] = 1
				svc_port_map[svc] = svc_obj.Spec.Ports[0].Port

				pod_list := &corev1.PodList{}
				err = r.Client.List(ctx, pod_list, client.InNamespace(req.Namespace), client.MatchingLabels(svc_obj.Spec.Selector))
				if err != nil {
					l.Error(err, "Unable to get pod list from matching labels")
					return ctrl.Result{}, err
				}
				// Now we need to get the deployment name from the pod name and then add the label to the deployment
				deployment_map := make(map[string]bool)
				for _, pod := range pod_list.Items {
					deployment_name, err := r.getDeploymentName(ctx, req, &pod)
					if err != nil {
						l.Error(err, "not able to get deployment name")
						return ctrl.Result{}, err
					}
					if deployment_map[deployment_name] == false {

						deployment_obj := &appsv1.Deployment{}
						err = r.Get(ctx, types.NamespacedName{
							Name:      deployment_name,
							Namespace: req.Namespace,
						}, deployment_obj)
						if err != nil {
							l.Error(err, "not able to fetch deployment")
							return ctrl.Result{}, err
						}
						pod_template_labels := deployment_obj.Spec.Template.Labels
						pod_template_labels["merge"] = name
						pod_template_labels["name"] = svc
						deployment_obj.Spec.Template.SetLabels(pod_template_labels)
						err = r.Update(ctx, deployment_obj)
						if err != nil {
							l.Error(err, "not able to delete label from deployment")
							return ctrl.Result{}, err
						}
						deployment_map[deployment_name] = true
					}
				}
			}

			time.Sleep(20 * time.Second) // sleep for some time to give time for the pods to restart
			// Now update the merged_pods array.
			merged_pods[name] = []string{}
			// merged pods can be found in the client list with matching label as merge=true
			merged_pod_list := &corev1.PodList{}
			err := r.Client.List(ctx, merged_pod_list, client.InNamespace(req.Namespace), client.MatchingLabels(map[string]string{"merge": name}))
			if err != nil {
				l.Error(err, "Unable to get pod list from matching labels")
				return ctrl.Result{}, err
			}
			for _, pod := range merged_pod_list.Items {
				merged_pods[name] = append(merged_pods[name], pod.Name)
			}

			// Now delete the services from the to_add list
			for _, svc := range to_add {
				service := &corev1.Service{}
				err = r.Get(ctx, types.NamespacedName{
					Name:      svc,
					Namespace: req.Namespace,
				}, service)
				if err != nil {
					l.Error(err, "not able to fetch service", "service", service)
					return ctrl.Result{}, err
				}
				err = r.Delete(ctx, service)
				if err != nil {
					l.Error(err, "could not delete service", "service", service)
					return ctrl.Result{}, err
				}
			}
			return ctrl.Result{}, err
		} else {

			fmt.Println("######################################### DELETION STARTED ###############################################")

			l.Info("Reconciler called for deletion of CRD")
			//We need to roll back the merge operation
			l.Info("Deleting the merged service.......")

			deployment_map := make(map[string]bool)
			for _, pod := range merged_pods[name] {

				pod_obj := &corev1.Pod{}
				err := r.Get(ctx, types.NamespacedName{
					Name:      pod,
					Namespace: req.Namespace,
				}, pod_obj)
				if err != nil {
					l.Info("It's coming to error", "pod", pod)
					continue
				}

				deployment_name, err := r.getDeploymentName(ctx, req, pod_obj)
				if err != nil {
					l.Error(err, "not able to get deployment name -- while rolling back")
					return ctrl.Result{}, err
				}

				fmt.Println("Deployment is ", deployment_name)
				if deployment_map[deployment_name] == false {

					deployment_obj := &appsv1.Deployment{}
					err = r.Get(ctx, types.NamespacedName{
						Name:      deployment_name,
						Namespace: req.Namespace,
					}, deployment_obj)
					if err != nil {
						l.Error(err, "not able to fetch deployment -- while rolling back")
						return ctrl.Result{}, err
					}
					pod_template_labels := deployment_obj.Spec.Template.Labels
					delete(pod_template_labels, "merge")
					deployment_obj.Spec.Template.SetLabels(pod_template_labels)
					err = r.Update(ctx, deployment_obj)
					if err != nil {
						l.Error(err, "not able to delete label from deployment -- while rolling back")
						return ctrl.Result{}, err
					}

					deployment_map[deployment_name] = true
				}

			}
			// Merge is rolled back. Delete merged svc & create old svc
			merged_svc_obj := &corev1.Service{}
			err := r.Get(ctx, types.NamespacedName{
				Name:      name,
				Namespace: req.Namespace,
			}, merged_svc_obj)
			if err != nil {
				l.Error(err, "Could not fetch merged svc for deletion -- while rolling back")
				return ctrl.Result{}, err
			}
			// Before deleting, delete the finalizer from merged service object.
			finalizer := "finalizer.newproj.controller.proj/" + name
			controllerutil.RemoveFinalizer(merged_svc_obj, finalizer)
			if err := r.Update(ctx, merged_svc_obj); err != nil {
				l.Info("error in removing finalizer from merged service")
				return ctrl.Result{}, err
			}
			err = r.Delete(ctx, merged_svc_obj)
			// Now create the old svc's
			for svc_name := range cur_mrgd_svcs_map[name] {

				var temp = make(map[string]string)
				temp["name"] = svc_name

				svc_obj := &corev1.Service{}
				svc_obj.Name = svc_name
				svc_obj.Namespace = req.Namespace
				svc_obj.Spec.Selector = temp
				svc_obj.Spec.Ports = []corev1.ServicePort{
					{
						Name:       "merged-service-port",
						Port:       svc_port_map[svc_name], // port number retrieved from the service port map
						Protocol:   corev1.ProtocolTCP,
						TargetPort: intstr.FromInt(8080),
					},
				}
				err = r.Create(ctx, svc_obj)
				if err != nil {
					l.Error(err, "Could not recreate old svc -- while rolling back")
					return ctrl.Result{}, err
				}
			}
			delete(merged_service_exists, name)
			delete(merged_pods, name)
			// delete all the svcs from svc_port_map which are in cur_mrgd_svcs_map[name]
			for svc_name := range cur_mrgd_svcs_map[name] {
				delete(svc_port_map, svc_name)
			}
			delete(cur_mrgd_svcs_map, name)

			return ctrl.Result{}, nil
		}
	}
}

// SetupWithManager sets up the controller with the Manager.
func (r *SvcMergerObjReconciler) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&newprojv1.SvcMergerObj{}).
		Complete(r)
}
