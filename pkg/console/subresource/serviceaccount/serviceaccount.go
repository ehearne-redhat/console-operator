package serviceaccount

import (
	"fmt"

	corev1 "k8s.io/api/core/v1"

	operatorv1 "github.com/openshift/api/operator/v1"
	"github.com/openshift/console-operator/bindata"
	"github.com/openshift/console-operator/pkg/console/subresource/util"
	"github.com/openshift/library-go/pkg/operator/resource/resourceread"
)

func DefaultDownloadsServiceAccount(operatorConfig *operatorv1.Console) *corev1.ServiceAccount {
	serviceAccount := resourceread.ReadServiceAccountV1OrDie(
		bindata.MustAsset("assets/serviceaccounts/downloads-sa.yaml"),
	)
	util.AddOwnerRef(serviceAccount, util.OwnerRefFrom(operatorConfig))
	return serviceAccount
}

func DefaultConsoleServiceAccount(operatorConfig *operatorv1.Console) *corev1.ServiceAccount {
	serviceAccount := resourceread.ReadServiceAccountV1OrDie(
		bindata.MustAsset("assets/serviceaccounts/console-sa.yaml"),
	)
	util.AddOwnerRef(serviceAccount, util.OwnerRefFrom(operatorConfig))
	return serviceAccount
}

// helper function to determine service account in controller
func DefaultServiceAccountFactory(serviceAccountName string, operatorConfig *operatorv1.Console) (*corev1.ServiceAccount, error) {
	switch serviceAccountName {
	case "downloads":
		return DefaultDownloadsServiceAccount(operatorConfig), nil
	case "console":
		return DefaultConsoleServiceAccount(operatorConfig), nil
	default:
		return nil, fmt.Errorf("No service account found for name %v .", serviceAccountName)
	}
}
