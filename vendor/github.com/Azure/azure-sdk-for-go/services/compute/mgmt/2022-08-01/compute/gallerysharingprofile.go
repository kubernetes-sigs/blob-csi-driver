package compute

// Copyright (c) Microsoft Corporation. All rights reserved.
// Licensed under the MIT License. See License.txt in the project root for license information.
//
// Code generated by Microsoft (R) AutoRest Code Generator.
// Changes may cause incorrect behavior and will be lost if the code is regenerated.

import (
	"context"
	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/azure"
	"github.com/Azure/go-autorest/tracing"
	"net/http"
)

// GallerySharingProfileClient is the compute Client
type GallerySharingProfileClient struct {
	BaseClient
}

// NewGallerySharingProfileClient creates an instance of the GallerySharingProfileClient client.
func NewGallerySharingProfileClient(subscriptionID string) GallerySharingProfileClient {
	return NewGallerySharingProfileClientWithBaseURI(DefaultBaseURI, subscriptionID)
}

// NewGallerySharingProfileClientWithBaseURI creates an instance of the GallerySharingProfileClient client using a
// custom endpoint.  Use this when interacting with an Azure cloud that uses a non-standard base URI (sovereign clouds,
// Azure stack).
func NewGallerySharingProfileClientWithBaseURI(baseURI string, subscriptionID string) GallerySharingProfileClient {
	return GallerySharingProfileClient{NewWithBaseURI(baseURI, subscriptionID)}
}

// Update update sharing profile of a gallery.
// Parameters:
// resourceGroupName - the name of the resource group.
// galleryName - the name of the Shared Image Gallery.
// sharingUpdate - parameters supplied to the update gallery sharing profile.
func (client GallerySharingProfileClient) Update(ctx context.Context, resourceGroupName string, galleryName string, sharingUpdate SharingUpdate) (result GallerySharingProfileUpdateFuture, err error) {
	if tracing.IsEnabled() {
		ctx = tracing.StartSpan(ctx, fqdn+"/GallerySharingProfileClient.Update")
		defer func() {
			sc := -1
			if result.FutureAPI != nil && result.FutureAPI.Response() != nil {
				sc = result.FutureAPI.Response().StatusCode
			}
			tracing.EndSpan(ctx, sc, err)
		}()
	}
	req, err := client.UpdatePreparer(ctx, resourceGroupName, galleryName, sharingUpdate)
	if err != nil {
		err = autorest.NewErrorWithError(err, "compute.GallerySharingProfileClient", "Update", nil, "Failure preparing request")
		return
	}

	result, err = client.UpdateSender(req)
	if err != nil {
		err = autorest.NewErrorWithError(err, "compute.GallerySharingProfileClient", "Update", result.Response(), "Failure sending request")
		return
	}

	return
}

// UpdatePreparer prepares the Update request.
func (client GallerySharingProfileClient) UpdatePreparer(ctx context.Context, resourceGroupName string, galleryName string, sharingUpdate SharingUpdate) (*http.Request, error) {
	pathParameters := map[string]interface{}{
		"galleryName":       autorest.Encode("path", galleryName),
		"resourceGroupName": autorest.Encode("path", resourceGroupName),
		"subscriptionId":    autorest.Encode("path", client.SubscriptionID),
	}

	const APIVersion = "2022-01-03"
	queryParameters := map[string]interface{}{
		"api-version": APIVersion,
	}

	preparer := autorest.CreatePreparer(
		autorest.AsContentType("application/json; charset=utf-8"),
		autorest.AsPost(),
		autorest.WithBaseURL(client.BaseURI),
		autorest.WithPathParameters("/subscriptions/{subscriptionId}/resourceGroups/{resourceGroupName}/providers/Microsoft.Compute/galleries/{galleryName}/share", pathParameters),
		autorest.WithJSON(sharingUpdate),
		autorest.WithQueryParameters(queryParameters))
	return preparer.Prepare((&http.Request{}).WithContext(ctx))
}

// UpdateSender sends the Update request. The method will close the
// http.Response Body if it receives an error.
func (client GallerySharingProfileClient) UpdateSender(req *http.Request) (future GallerySharingProfileUpdateFuture, err error) {
	var resp *http.Response
	future.FutureAPI = &azure.Future{}
	resp, err = client.Send(req, azure.DoRetryWithRegistration(client.Client))
	if err != nil {
		return
	}
	var azf azure.Future
	azf, err = azure.NewFutureFromResponse(resp)
	future.FutureAPI = &azf
	future.Result = future.result
	return
}

// UpdateResponder handles the response to the Update request. The method always
// closes the http.Response Body.
func (client GallerySharingProfileClient) UpdateResponder(resp *http.Response) (result SharingUpdate, err error) {
	err = autorest.Respond(
		resp,
		azure.WithErrorUnlessStatusCode(http.StatusOK, http.StatusAccepted),
		autorest.ByUnmarshallingJSON(&result),
		autorest.ByClosing())
	result.Response = autorest.Response{Response: resp}
	return
}