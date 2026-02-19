package serviceaccounts

import (
	"context"
	"fmt"
	"time"

	operatorv1 "github.com/openshift/api/operator/v1"
	configinformer "github.com/openshift/client-go/config/informers/externalversions"
	configlistersv1 "github.com/openshift/client-go/config/listers/config/v1"
	operatorinformerv1 "github.com/openshift/client-go/operator/informers/externalversions/operator/v1"
	operatorlistersv1 "github.com/openshift/client-go/operator/listers/operator/v1"

	"github.com/openshift/console-operator/pkg/api"
	"github.com/openshift/console-operator/pkg/console/controllers/util"
	"github.com/openshift/console-operator/pkg/console/status"
	serviceaccountssub "github.com/openshift/console-operator/pkg/console/subresource/serviceaccount"
	"github.com/openshift/library-go/pkg/controller/factory"
	"github.com/openshift/library-go/pkg/operator/events"
	"github.com/openshift/library-go/pkg/operator/resource/resourceapply"
	"github.com/openshift/library-go/pkg/operator/v1helpers"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	coreinformersv1 "k8s.io/client-go/informers/core/v1"
	coreclientv1 "k8s.io/client-go/kubernetes/typed/core/v1"

	"k8s.io/klog/v2"
)

type ServiceAccountSyncController struct {
	serviceAccountName string
	operatorClient     v1helpers.OperatorClient
	// configs
	consoleOperatorLister operatorlistersv1.ConsoleLister
	infrastructureLister  configlistersv1.InfrastructureLister
	// core kube
	serviceAccountClient coreclientv1.ServiceAccountsGetter
}

func NewServiceAccountSyncController(
	// clients
	operatorClient v1helpers.OperatorClient,
	// informer
	configInformer configinformer.SharedInformerFactory,
	operatorConfigInformer operatorinformerv1.ConsoleInformer,
	// core kube
	serviceAccountClient coreclientv1.ServiceAccountsGetter,
	serviceAccountInformer coreinformersv1.ServiceAccountInformer,
	// events
	recorder events.Recorder,
	// serviceAccountName
	serviceAccountName string,
	// controllerName,
	controllerName string,
	// controllerSuffix
	controllerSuffix string,
) factory.Controller {
	configV1Informers := configInformer.Config().V1()

	ctrl := &ServiceAccountSyncController{
		serviceAccountName: serviceAccountName,
		// configs
		operatorClient:        operatorClient,
		consoleOperatorLister: operatorConfigInformer.Lister(),
		infrastructureLister:  configInformer.Config().V1().Infrastructures().Lister(),
		// clients
		serviceAccountClient: serviceAccountClient,
	}

	configNameFilter := util.IncludeNamesFilter(api.ConfigResourceName)
	serviceAccountNameFilter := util.IncludeNamesFilter(serviceAccountName)

	return factory.New().
		WithFilteredEventsInformers( // infrastructure configs
			configNameFilter,
			operatorConfigInformer.Informer(),
			configV1Informers.Infrastructures().Informer(),
		).WithFilteredEventsInformers( // service account
		serviceAccountNameFilter,
		serviceAccountInformer.Informer(),
	).ResyncEvery(time.Minute).WithSync(ctrl.Sync).
		ToController(controllerName, recorder.WithComponentSuffix(controllerSuffix))
}

func (c *ServiceAccountSyncController) Sync(ctx context.Context, controllerContext factory.SyncContext) error {
	operatorConfig, err := c.consoleOperatorLister.Get(api.ConfigResourceName)
	if err != nil {
		return err
	}
	operatorConfigCopy := operatorConfig.DeepCopy()

	switch operatorConfigCopy.Spec.ManagementState {
	case operatorv1.Managed:
		klog.V(4).Infoln("console is in a managed state: syncing service account")
	case operatorv1.Unmanaged:
		klog.V(4).Infoln("console is in an unmanaged state: skipping service account sync")
		return nil
	case operatorv1.Removed:
		klog.V(4).Infoln("console is in a removed state: removing synced service account")
		return c.removeServiceAccount(ctx)
	default:
		return fmt.Errorf("unknown state: %v", operatorConfigCopy.Spec.ManagementState)
	}
	statusHandler := status.NewStatusHandler(c.operatorClient)

	_, _, serviceAccountErr := c.SyncServiceAccount(ctx, operatorConfigCopy, controllerContext)
	statusHandler.AddConditions(status.HandleProgressingOrDegraded("ServiceAccountSync", "FailedApply", serviceAccountErr))
	if serviceAccountErr != nil {
		return statusHandler.FlushAndReturn(serviceAccountErr)
	}

	return statusHandler.FlushAndReturn(nil)
}

func (c *ServiceAccountSyncController) removeServiceAccount(ctx context.Context) error {
	err := c.serviceAccountClient.ServiceAccounts(api.OpenShiftConsoleNamespace).Delete(ctx, c.serviceAccountName, metav1.DeleteOptions{})
	if apierrors.IsNotFound(err) {
		return nil
	}
	return err
}

func (c *ServiceAccountSyncController) SyncServiceAccount(ctx context.Context, operatorConfigCopy *operatorv1.Console, controllerContext factory.SyncContext) (*corev1.ServiceAccount, bool, error) {
	requiredServiceAccount, err := serviceaccountssub.DefaultServiceAccountFactory(c.serviceAccountName, operatorConfigCopy)
	if err != nil {
		return nil, false, err
	}

	// if the object has one or more ownerRef objects, then we must
	// ensure that their controller attribute is set to false.
	// Only one ownerRef.controller == true .
	// https://kubernetes.io/docs/concepts/overview/working-with-objects/owners-dependents/#owner-references-in-object-specifications

	for oR := range requiredServiceAccount.OwnerReferences {
		falseBool := false
		requiredServiceAccount.OwnerReferences[oR].Controller = &falseBool
	}

	return resourceapply.ApplyServiceAccount(ctx,
		c.serviceAccountClient,
		controllerContext.Recorder(),
		requiredServiceAccount,
	)
}
