// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.30.0
// 	protoc        v4.23.2
// source: github.com/Microsoft/hcsshim/internal/computeagent/computeagent.proto

package computeagent

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	anypb "google.golang.org/protobuf/types/known/anypb"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type AssignPCIInternalRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ContainerID          string `protobuf:"bytes,1,opt,name=container_id,json=containerId,proto3" json:"container_id,omitempty"`
	DeviceID             string `protobuf:"bytes,2,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"`
	VirtualFunctionIndex uint32 `protobuf:"varint,3,opt,name=virtual_function_index,json=virtualFunctionIndex,proto3" json:"virtual_function_index,omitempty"`
	NicID                string `protobuf:"bytes,4,opt,name=nic_id,json=nicId,proto3" json:"nic_id,omitempty"`
}

func (x *AssignPCIInternalRequest) Reset() {
	*x = AssignPCIInternalRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AssignPCIInternalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AssignPCIInternalRequest) ProtoMessage() {}

func (x *AssignPCIInternalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AssignPCIInternalRequest.ProtoReflect.Descriptor instead.
func (*AssignPCIInternalRequest) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{0}
}

func (x *AssignPCIInternalRequest) GetContainerID() string {
	if x != nil {
		return x.ContainerID
	}
	return ""
}

func (x *AssignPCIInternalRequest) GetDeviceID() string {
	if x != nil {
		return x.DeviceID
	}
	return ""
}

func (x *AssignPCIInternalRequest) GetVirtualFunctionIndex() uint32 {
	if x != nil {
		return x.VirtualFunctionIndex
	}
	return 0
}

func (x *AssignPCIInternalRequest) GetNicID() string {
	if x != nil {
		return x.NicID
	}
	return ""
}

type AssignPCIInternalResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ID string `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
}

func (x *AssignPCIInternalResponse) Reset() {
	*x = AssignPCIInternalResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AssignPCIInternalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AssignPCIInternalResponse) ProtoMessage() {}

func (x *AssignPCIInternalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AssignPCIInternalResponse.ProtoReflect.Descriptor instead.
func (*AssignPCIInternalResponse) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{1}
}

func (x *AssignPCIInternalResponse) GetID() string {
	if x != nil {
		return x.ID
	}
	return ""
}

type RemovePCIInternalRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ContainerID          string `protobuf:"bytes,1,opt,name=container_id,json=containerId,proto3" json:"container_id,omitempty"`
	DeviceID             string `protobuf:"bytes,2,opt,name=device_id,json=deviceId,proto3" json:"device_id,omitempty"`
	VirtualFunctionIndex uint32 `protobuf:"varint,3,opt,name=virtual_function_index,json=virtualFunctionIndex,proto3" json:"virtual_function_index,omitempty"`
}

func (x *RemovePCIInternalRequest) Reset() {
	*x = RemovePCIInternalRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RemovePCIInternalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RemovePCIInternalRequest) ProtoMessage() {}

func (x *RemovePCIInternalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RemovePCIInternalRequest.ProtoReflect.Descriptor instead.
func (*RemovePCIInternalRequest) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{2}
}

func (x *RemovePCIInternalRequest) GetContainerID() string {
	if x != nil {
		return x.ContainerID
	}
	return ""
}

func (x *RemovePCIInternalRequest) GetDeviceID() string {
	if x != nil {
		return x.DeviceID
	}
	return ""
}

func (x *RemovePCIInternalRequest) GetVirtualFunctionIndex() uint32 {
	if x != nil {
		return x.VirtualFunctionIndex
	}
	return 0
}

type RemovePCIInternalResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *RemovePCIInternalResponse) Reset() {
	*x = RemovePCIInternalResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *RemovePCIInternalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*RemovePCIInternalResponse) ProtoMessage() {}

func (x *RemovePCIInternalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use RemovePCIInternalResponse.ProtoReflect.Descriptor instead.
func (*RemovePCIInternalResponse) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{3}
}

type AddNICInternalRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ContainerID string     `protobuf:"bytes,1,opt,name=container_id,json=containerId,proto3" json:"container_id,omitempty"`
	NicID       string     `protobuf:"bytes,2,opt,name=nic_id,json=nicId,proto3" json:"nic_id,omitempty"`
	Endpoint    *anypb.Any `protobuf:"bytes,3,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
}

func (x *AddNICInternalRequest) Reset() {
	*x = AddNICInternalRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AddNICInternalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AddNICInternalRequest) ProtoMessage() {}

func (x *AddNICInternalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AddNICInternalRequest.ProtoReflect.Descriptor instead.
func (*AddNICInternalRequest) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{4}
}

func (x *AddNICInternalRequest) GetContainerID() string {
	if x != nil {
		return x.ContainerID
	}
	return ""
}

func (x *AddNICInternalRequest) GetNicID() string {
	if x != nil {
		return x.NicID
	}
	return ""
}

func (x *AddNICInternalRequest) GetEndpoint() *anypb.Any {
	if x != nil {
		return x.Endpoint
	}
	return nil
}

type AddNICInternalResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *AddNICInternalResponse) Reset() {
	*x = AddNICInternalResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AddNICInternalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AddNICInternalResponse) ProtoMessage() {}

func (x *AddNICInternalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AddNICInternalResponse.ProtoReflect.Descriptor instead.
func (*AddNICInternalResponse) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{5}
}

type ModifyNICInternalRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	NicID             string       `protobuf:"bytes,1,opt,name=nic_id,json=nicId,proto3" json:"nic_id,omitempty"`
	Endpoint          *anypb.Any   `protobuf:"bytes,2,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
	IovPolicySettings *IovSettings `protobuf:"bytes,3,opt,name=iov_policy_settings,json=iovPolicySettings,proto3" json:"iov_policy_settings,omitempty"`
}

func (x *ModifyNICInternalRequest) Reset() {
	*x = ModifyNICInternalRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ModifyNICInternalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ModifyNICInternalRequest) ProtoMessage() {}

func (x *ModifyNICInternalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ModifyNICInternalRequest.ProtoReflect.Descriptor instead.
func (*ModifyNICInternalRequest) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{6}
}

func (x *ModifyNICInternalRequest) GetNicID() string {
	if x != nil {
		return x.NicID
	}
	return ""
}

func (x *ModifyNICInternalRequest) GetEndpoint() *anypb.Any {
	if x != nil {
		return x.Endpoint
	}
	return nil
}

func (x *ModifyNICInternalRequest) GetIovPolicySettings() *IovSettings {
	if x != nil {
		return x.IovPolicySettings
	}
	return nil
}

type ModifyNICInternalResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ModifyNICInternalResponse) Reset() {
	*x = ModifyNICInternalResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[7]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ModifyNICInternalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ModifyNICInternalResponse) ProtoMessage() {}

func (x *ModifyNICInternalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[7]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ModifyNICInternalResponse.ProtoReflect.Descriptor instead.
func (*ModifyNICInternalResponse) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{7}
}

type DeleteNICInternalRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	ContainerID string     `protobuf:"bytes,1,opt,name=container_id,json=containerId,proto3" json:"container_id,omitempty"`
	NicID       string     `protobuf:"bytes,2,opt,name=nic_id,json=nicId,proto3" json:"nic_id,omitempty"`
	Endpoint    *anypb.Any `protobuf:"bytes,3,opt,name=endpoint,proto3" json:"endpoint,omitempty"`
}

func (x *DeleteNICInternalRequest) Reset() {
	*x = DeleteNICInternalRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[8]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DeleteNICInternalRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeleteNICInternalRequest) ProtoMessage() {}

func (x *DeleteNICInternalRequest) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[8]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeleteNICInternalRequest.ProtoReflect.Descriptor instead.
func (*DeleteNICInternalRequest) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{8}
}

func (x *DeleteNICInternalRequest) GetContainerID() string {
	if x != nil {
		return x.ContainerID
	}
	return ""
}

func (x *DeleteNICInternalRequest) GetNicID() string {
	if x != nil {
		return x.NicID
	}
	return ""
}

func (x *DeleteNICInternalRequest) GetEndpoint() *anypb.Any {
	if x != nil {
		return x.Endpoint
	}
	return nil
}

type DeleteNICInternalResponse struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *DeleteNICInternalResponse) Reset() {
	*x = DeleteNICInternalResponse{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[9]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DeleteNICInternalResponse) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeleteNICInternalResponse) ProtoMessage() {}

func (x *DeleteNICInternalResponse) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[9]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeleteNICInternalResponse.ProtoReflect.Descriptor instead.
func (*DeleteNICInternalResponse) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{9}
}

type IovSettings struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	IovOffloadWeight    uint32 `protobuf:"varint,1,opt,name=IovOffloadWeight,proto3" json:"IovOffloadWeight,omitempty"`
	QueuePairsRequested uint32 `protobuf:"varint,2,opt,name=QueuePairsRequested,proto3" json:"QueuePairsRequested,omitempty"`
	InterruptModeration uint32 `protobuf:"varint,3,opt,name=InterruptModeration,proto3" json:"InterruptModeration,omitempty"`
}

func (x *IovSettings) Reset() {
	*x = IovSettings{}
	if protoimpl.UnsafeEnabled {
		mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[10]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *IovSettings) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*IovSettings) ProtoMessage() {}

func (x *IovSettings) ProtoReflect() protoreflect.Message {
	mi := &file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[10]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use IovSettings.ProtoReflect.Descriptor instead.
func (*IovSettings) Descriptor() ([]byte, []int) {
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP(), []int{10}
}

func (x *IovSettings) GetIovOffloadWeight() uint32 {
	if x != nil {
		return x.IovOffloadWeight
	}
	return 0
}

func (x *IovSettings) GetQueuePairsRequested() uint32 {
	if x != nil {
		return x.QueuePairsRequested
	}
	return 0
}

func (x *IovSettings) GetInterruptModeration() uint32 {
	if x != nil {
		return x.InterruptModeration
	}
	return 0
}

var File_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto protoreflect.FileDescriptor

var file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDesc = []byte{
	0x0a, 0x45, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x4d, 0x69, 0x63,
	0x72, 0x6f, 0x73, 0x6f, 0x66, 0x74, 0x2f, 0x68, 0x63, 0x73, 0x73, 0x68, 0x69, 0x6d, 0x2f, 0x69,
	0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f, 0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x61,
	0x67, 0x65, 0x6e, 0x74, 0x2f, 0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x61, 0x67, 0x65, 0x6e,
	0x74, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x1a, 0x19, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2f,
	0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75, 0x66, 0x2f, 0x61, 0x6e, 0x79, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x22, 0xa7, 0x01, 0x0a, 0x18, 0x41, 0x73, 0x73, 0x69, 0x67, 0x6e, 0x50, 0x43, 0x49,
	0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12,
	0x21, 0x0a, 0x0c, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72,
	0x49, 0x64, 0x12, 0x1b, 0x0a, 0x09, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x64, 0x65, 0x76, 0x69, 0x63, 0x65, 0x49, 0x64, 0x12,
	0x34, 0x0a, 0x16, 0x76, 0x69, 0x72, 0x74, 0x75, 0x61, 0x6c, 0x5f, 0x66, 0x75, 0x6e, 0x63, 0x74,
	0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52,
	0x14, 0x76, 0x69, 0x72, 0x74, 0x75, 0x61, 0x6c, 0x46, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e,
	0x49, 0x6e, 0x64, 0x65, 0x78, 0x12, 0x15, 0x0a, 0x06, 0x6e, 0x69, 0x63, 0x5f, 0x69, 0x64, 0x18,
	0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6e, 0x69, 0x63, 0x49, 0x64, 0x22, 0x2b, 0x0a, 0x19,
	0x41, 0x73, 0x73, 0x69, 0x67, 0x6e, 0x50, 0x43, 0x49, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x22, 0x90, 0x01, 0x0a, 0x18, 0x52, 0x65,
	0x6d, 0x6f, 0x76, 0x65, 0x50, 0x43, 0x49, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x21, 0x0a, 0x0c, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69,
	0x6e, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x63, 0x6f,
	0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x49, 0x64, 0x12, 0x1b, 0x0a, 0x09, 0x64, 0x65, 0x76,
	0x69, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x64, 0x65,
	0x76, 0x69, 0x63, 0x65, 0x49, 0x64, 0x12, 0x34, 0x0a, 0x16, 0x76, 0x69, 0x72, 0x74, 0x75, 0x61,
	0x6c, 0x5f, 0x66, 0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x6e, 0x64, 0x65, 0x78,
	0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x14, 0x76, 0x69, 0x72, 0x74, 0x75, 0x61, 0x6c, 0x46,
	0x75, 0x6e, 0x63, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x6e, 0x64, 0x65, 0x78, 0x22, 0x1b, 0x0a, 0x19,
	0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x50, 0x43, 0x49, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x83, 0x01, 0x0a, 0x15, 0x41, 0x64,
	0x64, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x12, 0x21, 0x0a, 0x0c, 0x63, 0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72,
	0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x63, 0x6f, 0x6e, 0x74, 0x61,
	0x69, 0x6e, 0x65, 0x72, 0x49, 0x64, 0x12, 0x15, 0x0a, 0x06, 0x6e, 0x69, 0x63, 0x5f, 0x69, 0x64,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6e, 0x69, 0x63, 0x49, 0x64, 0x12, 0x30, 0x0a,
	0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x14, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x41, 0x6e, 0x79, 0x52, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x22,
	0x18, 0x0a, 0x16, 0x41, 0x64, 0x64, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0xa1, 0x01, 0x0a, 0x18, 0x4d, 0x6f,
	0x64, 0x69, 0x66, 0x79, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x15, 0x0a, 0x06, 0x6e, 0x69, 0x63, 0x5f, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6e, 0x69, 0x63, 0x49, 0x64, 0x12, 0x30, 0x0a,
	0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0b, 0x32,
	0x14, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f, 0x74, 0x6f, 0x62, 0x75,
	0x66, 0x2e, 0x41, 0x6e, 0x79, 0x52, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x12,
	0x3c, 0x0a, 0x13, 0x69, 0x6f, 0x76, 0x5f, 0x70, 0x6f, 0x6c, 0x69, 0x63, 0x79, 0x5f, 0x73, 0x65,
	0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x0c, 0x2e, 0x49,
	0x6f, 0x76, 0x53, 0x65, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x52, 0x11, 0x69, 0x6f, 0x76, 0x50,
	0x6f, 0x6c, 0x69, 0x63, 0x79, 0x53, 0x65, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x73, 0x22, 0x1b, 0x0a,
	0x19, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e,
	0x61, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x86, 0x01, 0x0a, 0x18, 0x44,
	0x65, 0x6c, 0x65, 0x74, 0x65, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x12, 0x21, 0x0a, 0x0c, 0x63, 0x6f, 0x6e, 0x74, 0x61,
	0x69, 0x6e, 0x65, 0x72, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0b, 0x63,
	0x6f, 0x6e, 0x74, 0x61, 0x69, 0x6e, 0x65, 0x72, 0x49, 0x64, 0x12, 0x15, 0x0a, 0x06, 0x6e, 0x69,
	0x63, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x6e, 0x69, 0x63, 0x49,
	0x64, 0x12, 0x30, 0x0a, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f, 0x69, 0x6e, 0x74, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x0b, 0x32, 0x14, 0x2e, 0x67, 0x6f, 0x6f, 0x67, 0x6c, 0x65, 0x2e, 0x70, 0x72, 0x6f,
	0x74, 0x6f, 0x62, 0x75, 0x66, 0x2e, 0x41, 0x6e, 0x79, 0x52, 0x08, 0x65, 0x6e, 0x64, 0x70, 0x6f,
	0x69, 0x6e, 0x74, 0x22, 0x1b, 0x0a, 0x19, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x4e, 0x49, 0x43,
	0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x22, 0x9d, 0x01, 0x0a, 0x0b, 0x49, 0x6f, 0x76, 0x53, 0x65, 0x74, 0x74, 0x69, 0x6e, 0x67, 0x73,
	0x12, 0x2a, 0x0a, 0x10, 0x49, 0x6f, 0x76, 0x4f, 0x66, 0x66, 0x6c, 0x6f, 0x61, 0x64, 0x57, 0x65,
	0x69, 0x67, 0x68, 0x74, 0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x10, 0x49, 0x6f, 0x76, 0x4f,
	0x66, 0x66, 0x6c, 0x6f, 0x61, 0x64, 0x57, 0x65, 0x69, 0x67, 0x68, 0x74, 0x12, 0x30, 0x0a, 0x13,
	0x51, 0x75, 0x65, 0x75, 0x65, 0x50, 0x61, 0x69, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73,
	0x74, 0x65, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x13, 0x51, 0x75, 0x65, 0x75, 0x65,
	0x50, 0x61, 0x69, 0x72, 0x73, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x65, 0x64, 0x12, 0x30,
	0x0a, 0x13, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x72, 0x75, 0x70, 0x74, 0x4d, 0x6f, 0x64, 0x65, 0x72,
	0x61, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x13, 0x49, 0x6e, 0x74,
	0x65, 0x72, 0x72, 0x75, 0x70, 0x74, 0x4d, 0x6f, 0x64, 0x65, 0x72, 0x61, 0x74, 0x69, 0x6f, 0x6e,
	0x32, 0xe3, 0x02, 0x0a, 0x0c, 0x43, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x41, 0x67, 0x65, 0x6e,
	0x74, 0x12, 0x3b, 0x0a, 0x06, 0x41, 0x64, 0x64, 0x4e, 0x49, 0x43, 0x12, 0x16, 0x2e, 0x41, 0x64,
	0x64, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75,
	0x65, 0x73, 0x74, 0x1a, 0x17, 0x2e, 0x41, 0x64, 0x64, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65,
	0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x44,
	0x0a, 0x09, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79, 0x4e, 0x49, 0x43, 0x12, 0x19, 0x2e, 0x4d, 0x6f,
	0x64, 0x69, 0x66, 0x79, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52,
	0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1a, 0x2e, 0x4d, 0x6f, 0x64, 0x69, 0x66, 0x79, 0x4e,
	0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e,
	0x73, 0x65, 0x22, 0x00, 0x12, 0x44, 0x0a, 0x09, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x4e, 0x49,
	0x43, 0x12, 0x19, 0x2e, 0x44, 0x65, 0x6c, 0x65, 0x74, 0x65, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74,
	0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1a, 0x2e, 0x44,
	0x65, 0x6c, 0x65, 0x74, 0x65, 0x4e, 0x49, 0x43, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c,
	0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x12, 0x44, 0x0a, 0x09, 0x41, 0x73,
	0x73, 0x69, 0x67, 0x6e, 0x50, 0x43, 0x49, 0x12, 0x19, 0x2e, 0x41, 0x73, 0x73, 0x69, 0x67, 0x6e,
	0x50, 0x43, 0x49, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x1a, 0x1a, 0x2e, 0x41, 0x73, 0x73, 0x69, 0x67, 0x6e, 0x50, 0x43, 0x49, 0x49, 0x6e,
	0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00,
	0x12, 0x44, 0x0a, 0x09, 0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x50, 0x43, 0x49, 0x12, 0x19, 0x2e,
	0x52, 0x65, 0x6d, 0x6f, 0x76, 0x65, 0x50, 0x43, 0x49, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61,
	0x6c, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74, 0x1a, 0x1a, 0x2e, 0x52, 0x65, 0x6d, 0x6f, 0x76,
	0x65, 0x50, 0x43, 0x49, 0x49, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x52, 0x65, 0x73, 0x70,
	0x6f, 0x6e, 0x73, 0x65, 0x22, 0x00, 0x42, 0x41, 0x5a, 0x3f, 0x67, 0x69, 0x74, 0x68, 0x75, 0x62,
	0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x4d, 0x69, 0x63, 0x72, 0x6f, 0x73, 0x6f, 0x66, 0x74, 0x2f, 0x68,
	0x63, 0x73, 0x73, 0x68, 0x69, 0x6d, 0x2f, 0x69, 0x6e, 0x74, 0x65, 0x72, 0x6e, 0x61, 0x6c, 0x2f,
	0x63, 0x6f, 0x6d, 0x70, 0x75, 0x74, 0x65, 0x61, 0x67, 0x65, 0x6e, 0x74, 0x3b, 0x63, 0x6f, 0x6d,
	0x70, 0x75, 0x74, 0x65, 0x61, 0x67, 0x65, 0x6e, 0x74, 0x62, 0x06, 0x70, 0x72, 0x6f, 0x74, 0x6f,
	0x33,
}

var (
	file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescOnce sync.Once
	file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescData = file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDesc
)

func file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescGZIP() []byte {
	file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescOnce.Do(func() {
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescData = protoimpl.X.CompressGZIP(file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescData)
	})
	return file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDescData
}

var file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_goTypes = []interface{}{
	(*AssignPCIInternalRequest)(nil),  // 0: AssignPCIInternalRequest
	(*AssignPCIInternalResponse)(nil), // 1: AssignPCIInternalResponse
	(*RemovePCIInternalRequest)(nil),  // 2: RemovePCIInternalRequest
	(*RemovePCIInternalResponse)(nil), // 3: RemovePCIInternalResponse
	(*AddNICInternalRequest)(nil),     // 4: AddNICInternalRequest
	(*AddNICInternalResponse)(nil),    // 5: AddNICInternalResponse
	(*ModifyNICInternalRequest)(nil),  // 6: ModifyNICInternalRequest
	(*ModifyNICInternalResponse)(nil), // 7: ModifyNICInternalResponse
	(*DeleteNICInternalRequest)(nil),  // 8: DeleteNICInternalRequest
	(*DeleteNICInternalResponse)(nil), // 9: DeleteNICInternalResponse
	(*IovSettings)(nil),               // 10: IovSettings
	(*anypb.Any)(nil),                 // 11: google.protobuf.Any
}
var file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_depIdxs = []int32{
	11, // 0: AddNICInternalRequest.endpoint:type_name -> google.protobuf.Any
	11, // 1: ModifyNICInternalRequest.endpoint:type_name -> google.protobuf.Any
	10, // 2: ModifyNICInternalRequest.iov_policy_settings:type_name -> IovSettings
	11, // 3: DeleteNICInternalRequest.endpoint:type_name -> google.protobuf.Any
	4,  // 4: ComputeAgent.AddNIC:input_type -> AddNICInternalRequest
	6,  // 5: ComputeAgent.ModifyNIC:input_type -> ModifyNICInternalRequest
	8,  // 6: ComputeAgent.DeleteNIC:input_type -> DeleteNICInternalRequest
	0,  // 7: ComputeAgent.AssignPCI:input_type -> AssignPCIInternalRequest
	2,  // 8: ComputeAgent.RemovePCI:input_type -> RemovePCIInternalRequest
	5,  // 9: ComputeAgent.AddNIC:output_type -> AddNICInternalResponse
	7,  // 10: ComputeAgent.ModifyNIC:output_type -> ModifyNICInternalResponse
	9,  // 11: ComputeAgent.DeleteNIC:output_type -> DeleteNICInternalResponse
	1,  // 12: ComputeAgent.AssignPCI:output_type -> AssignPCIInternalResponse
	3,  // 13: ComputeAgent.RemovePCI:output_type -> RemovePCIInternalResponse
	9,  // [9:14] is the sub-list for method output_type
	4,  // [4:9] is the sub-list for method input_type
	4,  // [4:4] is the sub-list for extension type_name
	4,  // [4:4] is the sub-list for extension extendee
	0,  // [0:4] is the sub-list for field type_name
}

func init() { file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_init() }
func file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_init() {
	if File_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AssignPCIInternalRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AssignPCIInternalResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RemovePCIInternalRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*RemovePCIInternalResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AddNICInternalRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AddNICInternalResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ModifyNICInternalRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[7].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ModifyNICInternalResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[8].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DeleteNICInternalRequest); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[9].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DeleteNICInternalResponse); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
		file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes[10].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*IovSettings); i {
			case 0:
				return &v.state
			case 1:
				return &v.sizeCache
			case 2:
				return &v.unknownFields
			default:
				return nil
			}
		}
	}
	type x struct{}
	out := protoimpl.TypeBuilder{
		File: protoimpl.DescBuilder{
			GoPackagePath: reflect.TypeOf(x{}).PkgPath(),
			RawDescriptor: file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_goTypes,
		DependencyIndexes: file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_depIdxs,
		MessageInfos:      file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_msgTypes,
	}.Build()
	File_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto = out.File
	file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_rawDesc = nil
	file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_goTypes = nil
	file_github_com_Microsoft_hcsshim_internal_computeagent_computeagent_proto_depIdxs = nil
}
