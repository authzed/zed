//go:build wasm
// +build wasm

package main

import (
	"context"

	v1 "github.com/authzed/authzed-go/proto/authzed/api/v1"
	"google.golang.org/grpc"
)

type wasmClient struct {
	v1.PermissionsServiceServer
	v1.SchemaServiceServer

	conn *grpc.ClientConn
}

func (wc wasmClient) ReadSchema(ctx context.Context, in *v1.ReadSchemaRequest, opts ...grpc.CallOption) (*v1.ReadSchemaResponse, error) {
	client := v1.NewSchemaServiceClient(wc.conn)
	return client.ReadSchema(ctx, in, opts...)
}

func (wc wasmClient) WriteSchema(ctx context.Context, in *v1.WriteSchemaRequest, opts ...grpc.CallOption) (*v1.WriteSchemaResponse, error) {
	client := v1.NewSchemaServiceClient(wc.conn)
	return client.WriteSchema(ctx, in, opts...)
}

func (wc wasmClient) WriteRelationships(ctx context.Context, in *v1.WriteRelationshipsRequest, opts ...grpc.CallOption) (*v1.WriteRelationshipsResponse, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.WriteRelationships(ctx, in, opts...)
}

func (wc wasmClient) DeleteRelationships(ctx context.Context, in *v1.DeleteRelationshipsRequest, opts ...grpc.CallOption) (*v1.DeleteRelationshipsResponse, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.DeleteRelationships(ctx, in, opts...)
}

func (wc wasmClient) CheckPermission(ctx context.Context, in *v1.CheckPermissionRequest, opts ...grpc.CallOption) (*v1.CheckPermissionResponse, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.CheckPermission(ctx, in, opts...)
}

func (wc wasmClient) CheckBulkPermissions(ctx context.Context, in *v1.CheckBulkPermissionsRequest, opts ...grpc.CallOption) (*v1.CheckBulkPermissionsResponse, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.CheckBulkPermissions(ctx, in, opts...)
}

func (wc wasmClient) ExpandPermissionTree(ctx context.Context, in *v1.ExpandPermissionTreeRequest, opts ...grpc.CallOption) (*v1.ExpandPermissionTreeResponse, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.ExpandPermissionTree(ctx, in, opts...)
}

func (wc wasmClient) ReadRelationships(ctx context.Context, in *v1.ReadRelationshipsRequest, opts ...grpc.CallOption) (v1.PermissionsService_ReadRelationshipsClient, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.ReadRelationships(ctx, in, opts...)
}

func (wc wasmClient) LookupResources(ctx context.Context, in *v1.LookupResourcesRequest, opts ...grpc.CallOption) (v1.PermissionsService_LookupResourcesClient, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.LookupResources(ctx, in, opts...)
}

func (wc wasmClient) LookupSubjects(ctx context.Context, in *v1.LookupSubjectsRequest, opts ...grpc.CallOption) (v1.PermissionsService_LookupSubjectsClient, error) {
	client := v1.NewPermissionsServiceClient(wc.conn)
	return client.LookupSubjects(ctx, in, opts...)
}

func (wc wasmClient) BulkExportRelationships(ctx context.Context, in *v1.BulkExportRelationshipsRequest, opts ...grpc.CallOption) (v1.ExperimentalService_BulkExportRelationshipsClient, error) {
	client := v1.NewExperimentalServiceClient(wc.conn)
	return client.BulkExportRelationships(ctx, in, opts...)
}

func (wc wasmClient) BulkImportRelationships(ctx context.Context, opts ...grpc.CallOption) (v1.ExperimentalService_BulkImportRelationshipsClient, error) {
	client := v1.NewExperimentalServiceClient(wc.conn)
	return client.BulkImportRelationships(ctx, opts...)
}

func (wc wasmClient) BulkCheckPermission(ctx context.Context, in *v1.BulkCheckPermissionRequest, opts ...grpc.CallOption) (*v1.BulkCheckPermissionResponse, error) {
	client := v1.NewExperimentalServiceClient(wc.conn)
	return client.BulkCheckPermission(ctx, in, opts...)
}

func (wc wasmClient) Watch(ctx context.Context, in *v1.WatchRequest, opts ...grpc.CallOption) (v1.WatchService_WatchClient, error) {
	client := v1.NewWatchServiceClient(wc.conn)
	return client.Watch(ctx, in, opts...)
}
