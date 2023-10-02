package azure

import (
	"context"
	"fmt"

	"github.com/Azure/azure-sdk-for-go/sdk/azidentity"
	"github.com/Azure/azure-sdk-for-go/sdk/resourcemanager/containerservice/armcontainerservice"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/weaveworks/cluster-reflector-controller/pkg/providers"
	"k8s.io/client-go/tools/clientcmd"
	clientcmdapi "k8s.io/client-go/tools/clientcmd/api"
)

type AzureProvider struct {
	SubscriptionID string
}

func NewAzureProvider(subscriptionID string) *AzureProvider {
	return &AzureProvider{
		SubscriptionID: subscriptionID,
	}
}

func (p *AzureProvider) ListClusters(ctx context.Context) ([]*providers.ProviderCluster, error) {
	cred, err := azidentity.NewDefaultAzureCredential(nil)
	if err != nil {
		return nil, fmt.Errorf("failed to obtain a credential: %v", err)
	}
	client, err := armcontainerservice.NewManagedClustersClient(p.SubscriptionID, cred, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create client: %v", err)
	}

	clusters := []*providers.ProviderCluster{}
	pager := client.NewListPager(nil)
	for pager.More() {
		nextResult, err := pager.NextPage(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to advance page: %v", err)
		}
		for _, aksCluster := range nextResult.Value {
			kubeConfig, err := getKubeconfigForCluster(ctx, client, aksCluster)
			if err != nil {
				return nil, fmt.Errorf("failed to get kubeconfig for cluster: %v", err)
			}

			clusters = append(clusters, &providers.ProviderCluster{
				Name:       *aksCluster.Name,
				KubeConfig: kubeConfig,
			})
		}
	}

	return clusters, nil
}

func getKubeconfigForCluster(ctx context.Context, client *armcontainerservice.ManagedClustersClient, aksCluster *armcontainerservice.ManagedCluster) (*clientcmdapi.Config, error) {

	resourceGroup, err := aksClusterResourceGroup(*aksCluster.ID)
	if err != nil {
		return nil, fmt.Errorf("failed to get cluster resource group: %v", err)
	}

	credentialsResponse, err := client.ListClusterAdminCredentials(ctx,
		resourceGroup,
		*aksCluster.Name,
		&armcontainerservice.ManagedClustersClientListClusterAdminCredentialsOptions{ServerFqdn: nil},
	)

	if err != nil {
		return nil, fmt.Errorf("failed to get cluster credentials: %v", err)
	}

	var kubeConfig *clientcmdapi.Config
	if credentialsResponse.Kubeconfigs != nil {
		kubeConfigBytes := credentialsResponse.Kubeconfigs[0].Value
		kubeConfig, err = clientcmd.Load(kubeConfigBytes)
		if err != nil {
			return nil, fmt.Errorf("failed to load kubeconfig: %v", err)
		}
	}

	return kubeConfig, nil
}

func aksClusterResourceGroup(clusterID string) (string, error) {
	resource, err := azure.ParseResourceID(clusterID)
	if err != nil {
		return "", fmt.Errorf("failed to parse cluster resource group from id: %v", err)
	}
	return resource.ResourceGroup, nil
}