/*
Copyright 2019 The Kubernetes Authors.

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

package azure

import (
	"context"
	"fmt"

	"k8s.io/utils/pointer"
	"sigs.k8s.io/blob-csi-driver/test/utils/credentials"

	"github.com/Azure/azure-sdk-for-go/services/preview/authorization/mgmt/2018-01-01-preview/authorization"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/adal"
	"github.com/Azure/go-autorest/autorest/azure"
	uuid "github.com/satori/go.uuid"
)

type AuthorizationClient struct {
	cred *credentials.Credentials
}

func NewAuthorizationClient() (*AuthorizationClient, error) {
	e2eCred, err := credentials.ParseAzureCredentialFile()
	if err != nil {
		return nil, err
	}

	return &AuthorizationClient{
		cred: &credentials.Credentials{
			SubscriptionID:  e2eCred.SubscriptionID,
			ResourceGroup:   e2eCred.ResourceGroup,
			Location:        e2eCred.Location,
			TenantID:        e2eCred.TenantID,
			Cloud:           e2eCred.Cloud,
			AADClientID:     e2eCred.AADClientID,
			AADClientSecret: e2eCred.AADClientSecret,
		},
	}, nil
}

func (a *AuthorizationClient) AssignRole(ctx context.Context, resourceID, principalID, roleDefID string) (authorization.RoleAssignment, error) {
	roleAssignmentsClient, err := a.getRoleAssignmentsClient()
	if err != nil {
		return authorization.RoleAssignment{}, err
	}

	return roleAssignmentsClient.Create(
		ctx,
		resourceID,
		uuid.NewV1().String(),
		authorization.RoleAssignmentCreateParameters{
			RoleAssignmentProperties: &authorization.RoleAssignmentProperties{
				PrincipalID:      pointer.String(principalID),
				RoleDefinitionID: pointer.String(roleDefID),
			},
		})
}

func (a *AuthorizationClient) DeleteRoleAssignment(ctx context.Context, id string) (authorization.RoleAssignment, error) {
	roleAssignmentsClient, err := a.getRoleAssignmentsClient()
	if err != nil {
		return authorization.RoleAssignment{}, err
	}

	return roleAssignmentsClient.DeleteByID(ctx, id)
}

func (a *AuthorizationClient) GetRoleDefinition(ctx context.Context, resourceID, roleName string) (authorization.RoleDefinition, error) {
	roleDefClient, err := a.getRoleDefinitionsClient()
	if err != nil {
		return authorization.RoleDefinition{}, err
	}

	filter := fmt.Sprintf("roleName eq '%s'", roleName)
	list, err := roleDefClient.List(ctx, resourceID, filter)
	if err != nil {
		return authorization.RoleDefinition{}, err
	}

	return list.Values()[0], nil
}

func (a *AuthorizationClient) getRoleAssignmentsClient() (*authorization.RoleAssignmentsClient, error) {
	roleClient := authorization.NewRoleAssignmentsClient(a.cred.SubscriptionID)

	env, err := azure.EnvironmentFromName(a.cred.Cloud)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, a.cred.TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(*oauthConfig, a.cred.AADClientID, a.cred.AADClientSecret, env.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(token)

	roleClient.Authorizer = authorizer

	return &roleClient, nil
}

func (a *AuthorizationClient) getRoleDefinitionsClient() (*authorization.RoleDefinitionsClient, error) {
	roleDefClient := authorization.NewRoleDefinitionsClient(a.cred.SubscriptionID)

	env, err := azure.EnvironmentFromName(a.cred.Cloud)
	if err != nil {
		return nil, err
	}

	oauthConfig, err := adal.NewOAuthConfig(env.ActiveDirectoryEndpoint, a.cred.TenantID)
	if err != nil {
		return nil, err
	}

	token, err := adal.NewServicePrincipalToken(*oauthConfig, a.cred.AADClientID, a.cred.AADClientSecret, env.ResourceManagerEndpoint)
	if err != nil {
		return nil, err
	}

	authorizer := autorest.NewBearerAuthorizer(token)

	roleDefClient.Authorizer = authorizer

	return &roleDefClient, nil
}
