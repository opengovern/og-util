// Code generated by protoc-gen-go. DO NOT EDIT.
// versions:
// 	protoc-gen-go v1.28.1
// 	protoc        v3.6.1
// source: pkg/describe/proto/describe.proto

package golang

import (
	protoreflect "google.golang.org/protobuf/reflect/protoreflect"
	protoimpl "google.golang.org/protobuf/runtime/protoimpl"
	reflect "reflect"
	sync "sync"
)

const (
	// Verify that this generated code is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(20 - protoimpl.MinVersion)
	// Verify that runtime/protoimpl is sufficiently up-to-date.
	_ = protoimpl.EnforceVersion(protoimpl.MaxVersion - 20)
)

type DescribeJob struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	JobId         uint32 `protobuf:"varint,1,opt,name=job_id,json=jobId,proto3" json:"job_id,omitempty"`
	ScheduleJobId uint32 `protobuf:"varint,2,opt,name=schedule_job_id,json=scheduleJobId,proto3" json:"schedule_job_id,omitempty"`
	ParentJobId   uint32 `protobuf:"varint,3,opt,name=parent_job_id,json=parentJobId,proto3" json:"parent_job_id,omitempty"`
	ResourceType  string `protobuf:"bytes,4,opt,name=resource_type,json=resourceType,proto3" json:"resource_type,omitempty"`
	SourceId      string `protobuf:"bytes,5,opt,name=source_id,json=sourceId,proto3" json:"source_id,omitempty"`
	AccountId     string `protobuf:"bytes,6,opt,name=account_id,json=accountId,proto3" json:"account_id,omitempty"`
	DescribedAt   int64  `protobuf:"varint,7,opt,name=described_at,json=describedAt,proto3" json:"described_at,omitempty"`
	SourceType    string `protobuf:"bytes,8,opt,name=source_type,json=sourceType,proto3" json:"source_type,omitempty"`
	ConfigReg     string `protobuf:"bytes,9,opt,name=config_reg,json=configReg,proto3" json:"config_reg,omitempty"`
	TriggerType   string `protobuf:"bytes,10,opt,name=trigger_type,json=triggerType,proto3" json:"trigger_type,omitempty"`
	RetryCounter  uint32 `protobuf:"varint,11,opt,name=retry_counter,json=retryCounter,proto3" json:"retry_counter,omitempty"`
}

func (x *DescribeJob) Reset() {
	*x = DescribeJob{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[0]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DescribeJob) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DescribeJob) ProtoMessage() {}

func (x *DescribeJob) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[0]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DescribeJob.ProtoReflect.Descriptor instead.
func (*DescribeJob) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{0}
}

func (x *DescribeJob) GetJobId() uint32 {
	if x != nil {
		return x.JobId
	}
	return 0
}

func (x *DescribeJob) GetScheduleJobId() uint32 {
	if x != nil {
		return x.ScheduleJobId
	}
	return 0
}

func (x *DescribeJob) GetParentJobId() uint32 {
	if x != nil {
		return x.ParentJobId
	}
	return 0
}

func (x *DescribeJob) GetResourceType() string {
	if x != nil {
		return x.ResourceType
	}
	return ""
}

func (x *DescribeJob) GetSourceId() string {
	if x != nil {
		return x.SourceId
	}
	return ""
}

func (x *DescribeJob) GetAccountId() string {
	if x != nil {
		return x.AccountId
	}
	return ""
}

func (x *DescribeJob) GetDescribedAt() int64 {
	if x != nil {
		return x.DescribedAt
	}
	return 0
}

func (x *DescribeJob) GetSourceType() string {
	if x != nil {
		return x.SourceType
	}
	return ""
}

func (x *DescribeJob) GetConfigReg() string {
	if x != nil {
		return x.ConfigReg
	}
	return ""
}

func (x *DescribeJob) GetTriggerType() string {
	if x != nil {
		return x.TriggerType
	}
	return ""
}

func (x *DescribeJob) GetRetryCounter() uint32 {
	if x != nil {
		return x.RetryCounter
	}
	return 0
}

type AWSResources struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Resources []*AWSResource `protobuf:"bytes,1,rep,name=resources,proto3" json:"resources,omitempty"`
}

func (x *AWSResources) Reset() {
	*x = AWSResources{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[1]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AWSResources) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AWSResources) ProtoMessage() {}

func (x *AWSResources) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[1]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AWSResources.ProtoReflect.Descriptor instead.
func (*AWSResources) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{1}
}

func (x *AWSResources) GetResources() []*AWSResource {
	if x != nil {
		return x.Resources
	}
	return nil
}

type AWSResource struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Arn             string            `protobuf:"bytes,1,opt,name=arn,proto3" json:"arn,omitempty"`
	Id              string            `protobuf:"bytes,2,opt,name=id,proto3" json:"id,omitempty"`
	Name            string            `protobuf:"bytes,3,opt,name=name,proto3" json:"name,omitempty"`
	Account         string            `protobuf:"bytes,4,opt,name=account,proto3" json:"account,omitempty"`
	Region          string            `protobuf:"bytes,5,opt,name=region,proto3" json:"region,omitempty"`
	Partition       string            `protobuf:"bytes,6,opt,name=partition,proto3" json:"partition,omitempty"`
	Type            string            `protobuf:"bytes,7,opt,name=type,proto3" json:"type,omitempty"`
	DescriptionJson string       `protobuf:"bytes,8,opt,name=description_json,json=descriptionJson,proto3" json:"description_json,omitempty"`
	Job             *DescribeJob `protobuf:"bytes,9,opt,name=job,proto3" json:"job,omitempty"`
	UniqueId        string       `protobuf:"bytes,10,opt,name=unique_id,json=uniqueId,proto3" json:"unique_id,omitempty"`
	Metadata        map[string]string `protobuf:"bytes,11,rep,name=metadata,proto3" json:"metadata,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Tags            map[string]string `protobuf:"bytes,12,rep,name=tags,proto3" json:"tags,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *AWSResource) Reset() {
	*x = AWSResource{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[2]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AWSResource) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AWSResource) ProtoMessage() {}

func (x *AWSResource) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[2]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AWSResource.ProtoReflect.Descriptor instead.
func (*AWSResource) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{2}
}

func (x *AWSResource) GetArn() string {
	if x != nil {
		return x.Arn
	}
	return ""
}

func (x *AWSResource) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *AWSResource) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *AWSResource) GetAccount() string {
	if x != nil {
		return x.Account
	}
	return ""
}

func (x *AWSResource) GetRegion() string {
	if x != nil {
		return x.Region
	}
	return ""
}

func (x *AWSResource) GetPartition() string {
	if x != nil {
		return x.Partition
	}
	return ""
}

func (x *AWSResource) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *AWSResource) GetDescriptionJson() string {
	if x != nil {
		return x.DescriptionJson
	}
	return ""
}

func (x *AWSResource) GetJob() *DescribeJob {
	if x != nil {
		return x.Job
	}
	return nil
}

func (x *AWSResource) GetUniqueId() string {
	if x != nil {
		return x.UniqueId
	}
	return ""
}

func (x *AWSResource) GetMetadata() map[string]string {
	if x != nil {
		return x.Metadata
	}
	return nil
}

func (x *AWSResource) GetTags() map[string]string {
	if x != nil {
		return x.Tags
	}
	return nil
}

type AzureResources struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Resources []*AzureResource `protobuf:"bytes,1,rep,name=resources,proto3" json:"resources,omitempty"`
}

func (x *AzureResources) Reset() {
	*x = AzureResources{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[3]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AzureResources) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AzureResources) ProtoMessage() {}

func (x *AzureResources) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[3]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AzureResources.ProtoReflect.Descriptor instead.
func (*AzureResources) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{3}
}

func (x *AzureResources) GetResources() []*AzureResource {
	if x != nil {
		return x.Resources
	}
	return nil
}

type AzureResource struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	Id              string            `protobuf:"bytes,1,opt,name=id,proto3" json:"id,omitempty"`
	Name            string            `protobuf:"bytes,2,opt,name=name,proto3" json:"name,omitempty"`
	Type            string            `protobuf:"bytes,3,opt,name=type,proto3" json:"type,omitempty"`
	ResourceGroup   string            `protobuf:"bytes,4,opt,name=resource_group,json=resourceGroup,proto3" json:"resource_group,omitempty"`
	Location        string            `protobuf:"bytes,5,opt,name=location,proto3" json:"location,omitempty"`
	SubscriptionId  string            `protobuf:"bytes,6,opt,name=subscription_id,json=subscriptionId,proto3" json:"subscription_id,omitempty"`
	DescriptionJson string       `protobuf:"bytes,7,opt,name=description_json,json=descriptionJson,proto3" json:"description_json,omitempty"`
	Job             *DescribeJob `protobuf:"bytes,8,opt,name=job,proto3" json:"job,omitempty"`
	UniqueId        string       `protobuf:"bytes,9,opt,name=unique_id,json=uniqueId,proto3" json:"unique_id,omitempty"`
	Metadata        map[string]string `protobuf:"bytes,10,rep,name=metadata,proto3" json:"metadata,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
	Tags            map[string]string `protobuf:"bytes,11,rep,name=tags,proto3" json:"tags,omitempty" protobuf_key:"bytes,1,opt,name=key,proto3" protobuf_val:"bytes,2,opt,name=value,proto3"`
}

func (x *AzureResource) Reset() {
	*x = AzureResource{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[4]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *AzureResource) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*AzureResource) ProtoMessage() {}

func (x *AzureResource) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[4]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use AzureResource.ProtoReflect.Descriptor instead.
func (*AzureResource) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{4}
}

func (x *AzureResource) GetId() string {
	if x != nil {
		return x.Id
	}
	return ""
}

func (x *AzureResource) GetName() string {
	if x != nil {
		return x.Name
	}
	return ""
}

func (x *AzureResource) GetType() string {
	if x != nil {
		return x.Type
	}
	return ""
}

func (x *AzureResource) GetResourceGroup() string {
	if x != nil {
		return x.ResourceGroup
	}
	return ""
}

func (x *AzureResource) GetLocation() string {
	if x != nil {
		return x.Location
	}
	return ""
}

func (x *AzureResource) GetSubscriptionId() string {
	if x != nil {
		return x.SubscriptionId
	}
	return ""
}

func (x *AzureResource) GetDescriptionJson() string {
	if x != nil {
		return x.DescriptionJson
	}
	return ""
}

func (x *AzureResource) GetJob() *DescribeJob {
	if x != nil {
		return x.Job
	}
	return nil
}

func (x *AzureResource) GetUniqueId() string {
	if x != nil {
		return x.UniqueId
	}
	return ""
}

func (x *AzureResource) GetMetadata() map[string]string {
	if x != nil {
		return x.Metadata
	}
	return nil
}

func (x *AzureResource) GetTags() map[string]string {
	if x != nil {
		return x.Tags
	}
	return nil
}

type DeliverResultRequest struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields

	JobId                uint32       `protobuf:"varint,1,opt,name=job_id,json=jobId,proto3" json:"job_id,omitempty"`
	ParentJobId          uint32       `protobuf:"varint,2,opt,name=parent_job_id,json=parentJobId,proto3" json:"parent_job_id,omitempty"`
	Status               string       `protobuf:"bytes,3,opt,name=status,proto3" json:"status,omitempty"`
	Error                string       `protobuf:"bytes,4,opt,name=error,proto3" json:"error,omitempty"`
	DescribeJob          *DescribeJob `protobuf:"bytes,5,opt,name=describe_job,json=describeJob,proto3" json:"describe_job,omitempty"`
	DescribedResourceIds []string     `protobuf:"bytes,6,rep,name=described_resource_ids,json=describedResourceIds,proto3" json:"described_resource_ids,omitempty"`
}

func (x *DeliverResultRequest) Reset() {
	*x = DeliverResultRequest{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[5]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *DeliverResultRequest) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*DeliverResultRequest) ProtoMessage() {}

func (x *DeliverResultRequest) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[5]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use DeliverResultRequest.ProtoReflect.Descriptor instead.
func (*DeliverResultRequest) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{5}
}

func (x *DeliverResultRequest) GetJobId() uint32 {
	if x != nil {
		return x.JobId
	}
	return 0
}

func (x *DeliverResultRequest) GetParentJobId() uint32 {
	if x != nil {
		return x.ParentJobId
	}
	return 0
}

func (x *DeliverResultRequest) GetStatus() string {
	if x != nil {
		return x.Status
	}
	return ""
}

func (x *DeliverResultRequest) GetError() string {
	if x != nil {
		return x.Error
	}
	return ""
}

func (x *DeliverResultRequest) GetDescribeJob() *DescribeJob {
	if x != nil {
		return x.DescribeJob
	}
	return nil
}

func (x *DeliverResultRequest) GetDescribedResourceIds() []string {
	if x != nil {
		return x.DescribedResourceIds
	}
	return nil
}

type ResponseOK struct {
	state         protoimpl.MessageState
	sizeCache     protoimpl.SizeCache
	unknownFields protoimpl.UnknownFields
}

func (x *ResponseOK) Reset() {
	*x = ResponseOK{}
	if protoimpl.UnsafeEnabled {
		mi := &file_pkg_describe_proto_describe_proto_msgTypes[6]
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		ms.StoreMessageInfo(mi)
	}
}

func (x *ResponseOK) String() string {
	return protoimpl.X.MessageStringOf(x)
}

func (*ResponseOK) ProtoMessage() {}

func (x *ResponseOK) ProtoReflect() protoreflect.Message {
	mi := &file_pkg_describe_proto_describe_proto_msgTypes[6]
	if protoimpl.UnsafeEnabled && x != nil {
		ms := protoimpl.X.MessageStateOf(protoimpl.Pointer(x))
		if ms.LoadMessageInfo() == nil {
			ms.StoreMessageInfo(mi)
		}
		return ms
	}
	return mi.MessageOf(x)
}

// Deprecated: Use ResponseOK.ProtoReflect.Descriptor instead.
func (*ResponseOK) Descriptor() ([]byte, []int) {
	return file_pkg_describe_proto_describe_proto_rawDescGZIP(), []int{6}
}

var File_pkg_describe_proto_describe_proto protoreflect.FileDescriptor

var file_pkg_describe_proto_describe_proto_rawDesc = []byte{
	0x0a, 0x21, 0x70, 0x6b, 0x67, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2f, 0x70,
	0x72, 0x6f, 0x74, 0x6f, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x12, 0x11, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72,
	0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x22, 0xfc, 0x02, 0x0a, 0x0b, 0x44, 0x65, 0x73, 0x63, 0x72,
	0x69, 0x62, 0x65, 0x4a, 0x6f, 0x62, 0x12, 0x15, 0x0a, 0x06, 0x6a, 0x6f, 0x62, 0x5f, 0x69, 0x64,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x05, 0x6a, 0x6f, 0x62, 0x49, 0x64, 0x12, 0x26, 0x0a,
	0x0f, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65, 0x5f, 0x6a, 0x6f, 0x62, 0x5f, 0x69, 0x64,
	0x18, 0x02, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0d, 0x73, 0x63, 0x68, 0x65, 0x64, 0x75, 0x6c, 0x65,
	0x4a, 0x6f, 0x62, 0x49, 0x64, 0x12, 0x22, 0x0a, 0x0d, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x5f,
	0x6a, 0x6f, 0x62, 0x5f, 0x69, 0x64, 0x18, 0x03, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0b, 0x70, 0x61,
	0x72, 0x65, 0x6e, 0x74, 0x4a, 0x6f, 0x62, 0x49, 0x64, 0x12, 0x23, 0x0a, 0x0d, 0x72, 0x65, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x0c, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1b,
	0x0a, 0x09, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x05, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x08, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x49, 0x64, 0x12, 0x1d, 0x0a, 0x0a, 0x61,
	0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x5f, 0x69, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52,
	0x09, 0x61, 0x63, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x49, 0x64, 0x12, 0x21, 0x0a, 0x0c, 0x64, 0x65,
	0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x64, 0x5f, 0x61, 0x74, 0x18, 0x07, 0x20, 0x01, 0x28, 0x03,
	0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x64, 0x41, 0x74, 0x12, 0x1f, 0x0a,
	0x0b, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x08, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x0a, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x54, 0x79, 0x70, 0x65, 0x12, 0x1d,
	0x0a, 0x0a, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x5f, 0x72, 0x65, 0x67, 0x18, 0x09, 0x20, 0x01,
	0x28, 0x09, 0x52, 0x09, 0x63, 0x6f, 0x6e, 0x66, 0x69, 0x67, 0x52, 0x65, 0x67, 0x12, 0x21, 0x0a,
	0x0c, 0x74, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x5f, 0x74, 0x79, 0x70, 0x65, 0x18, 0x0a, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x0b, 0x74, 0x72, 0x69, 0x67, 0x67, 0x65, 0x72, 0x54, 0x79, 0x70, 0x65,
	0x12, 0x23, 0x0a, 0x0d, 0x72, 0x65, 0x74, 0x72, 0x79, 0x5f, 0x63, 0x6f, 0x75, 0x6e, 0x74, 0x65,
	0x72, 0x18, 0x0b, 0x20, 0x01, 0x28, 0x0d, 0x52, 0x0c, 0x72, 0x65, 0x74, 0x72, 0x79, 0x43, 0x6f,
	0x75, 0x6e, 0x74, 0x65, 0x72, 0x22, 0x4c, 0x0a, 0x0c, 0x41, 0x57, 0x53, 0x52, 0x65, 0x73, 0x6f,
	0x75, 0x72, 0x63, 0x65, 0x73, 0x12, 0x3c, 0x0a, 0x09, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63,
	0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75,
	0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x57, 0x53,
	0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x09, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x73, 0x22, 0x9f, 0x04, 0x0a, 0x0b, 0x41, 0x57, 0x53, 0x52, 0x65, 0x73, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x12, 0x10, 0x0a, 0x03, 0x61, 0x72, 0x6e, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09,
	0x52, 0x03, 0x61, 0x72, 0x6e, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x18, 0x03, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x18, 0x0a, 0x07, 0x61, 0x63, 0x63,
	0x6f, 0x75, 0x6e, 0x74, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x07, 0x61, 0x63, 0x63, 0x6f,
	0x75, 0x6e, 0x74, 0x12, 0x16, 0x0a, 0x06, 0x72, 0x65, 0x67, 0x69, 0x6f, 0x6e, 0x18, 0x05, 0x20,
	0x01, 0x28, 0x09, 0x52, 0x06, 0x72, 0x65, 0x67, 0x69, 0x6f, 0x6e, 0x12, 0x1c, 0x0a, 0x09, 0x70,
	0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x09,
	0x70, 0x61, 0x72, 0x74, 0x69, 0x74, 0x69, 0x6f, 0x6e, 0x12, 0x12, 0x0a, 0x04, 0x74, 0x79, 0x70,
	0x65, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x79, 0x70, 0x65, 0x12, 0x29, 0x0a,
	0x10, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x6a, 0x73, 0x6f,
	0x6e, 0x18, 0x08, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x4a, 0x73, 0x6f, 0x6e, 0x12, 0x30, 0x0a, 0x03, 0x6a, 0x6f, 0x62, 0x18,
	0x09, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65,
	0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69,
	0x62, 0x65, 0x4a, 0x6f, 0x62, 0x52, 0x03, 0x6a, 0x6f, 0x62, 0x12, 0x1b, 0x0a, 0x09, 0x75, 0x6e,
	0x69, 0x71, 0x75, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x0a, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x75,
	0x6e, 0x69, 0x71, 0x75, 0x65, 0x49, 0x64, 0x12, 0x48, 0x0a, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x18, 0x0b, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2c, 0x2e, 0x6b, 0x61, 0x79, 0x74,
	0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x57,
	0x53, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61,
	0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74,
	0x61, 0x12, 0x3c, 0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x0c, 0x20, 0x03, 0x28, 0x0b, 0x32,
	0x28, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65,
	0x2e, 0x76, 0x31, 0x2e, 0x41, 0x57, 0x53, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e,
	0x54, 0x61, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x04, 0x74, 0x61, 0x67, 0x73, 0x1a,
	0x3b, 0x0a, 0x0d, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79,
	0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b,
	0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x1a, 0x37, 0x0a, 0x09,
	0x54, 0x61, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79,
	0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76,
	0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75,
	0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0x50, 0x0a, 0x0e, 0x41, 0x7a, 0x75, 0x72, 0x65, 0x52, 0x65,
	0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x12, 0x3e, 0x0a, 0x09, 0x72, 0x65, 0x73, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x73, 0x18, 0x01, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x20, 0x2e, 0x6b, 0x61, 0x79,
	0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41,
	0x7a, 0x75, 0x72, 0x65, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x52, 0x09, 0x72, 0x65,
	0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x22, 0xaf, 0x04, 0x0a, 0x0d, 0x41, 0x7a, 0x75, 0x72,
	0x65, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x12, 0x0e, 0x0a, 0x02, 0x69, 0x64, 0x18,
	0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x02, 0x69, 0x64, 0x12, 0x12, 0x0a, 0x04, 0x6e, 0x61, 0x6d,
	0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x6e, 0x61, 0x6d, 0x65, 0x12, 0x12, 0x0a,
	0x04, 0x74, 0x79, 0x70, 0x65, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x04, 0x74, 0x79, 0x70,
	0x65, 0x12, 0x25, 0x0a, 0x0e, 0x72, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x5f, 0x67, 0x72,
	0x6f, 0x75, 0x70, 0x18, 0x04, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0d, 0x72, 0x65, 0x73, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x47, 0x72, 0x6f, 0x75, 0x70, 0x12, 0x1a, 0x0a, 0x08, 0x6c, 0x6f, 0x63, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x18, 0x05, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x6c, 0x6f, 0x63, 0x61,
	0x74, 0x69, 0x6f, 0x6e, 0x12, 0x27, 0x0a, 0x0f, 0x73, 0x75, 0x62, 0x73, 0x63, 0x72, 0x69, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x69, 0x64, 0x18, 0x06, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0e, 0x73,
	0x75, 0x62, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x49, 0x64, 0x12, 0x29, 0x0a,
	0x10, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70, 0x74, 0x69, 0x6f, 0x6e, 0x5f, 0x6a, 0x73, 0x6f,
	0x6e, 0x18, 0x07, 0x20, 0x01, 0x28, 0x09, 0x52, 0x0f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x70,
	0x74, 0x69, 0x6f, 0x6e, 0x4a, 0x73, 0x6f, 0x6e, 0x12, 0x30, 0x0a, 0x03, 0x6a, 0x6f, 0x62, 0x18,
	0x08, 0x20, 0x01, 0x28, 0x0b, 0x32, 0x1e, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65,
	0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69,
	0x62, 0x65, 0x4a, 0x6f, 0x62, 0x52, 0x03, 0x6a, 0x6f, 0x62, 0x12, 0x1b, 0x0a, 0x09, 0x75, 0x6e,
	0x69, 0x71, 0x75, 0x65, 0x5f, 0x69, 0x64, 0x18, 0x09, 0x20, 0x01, 0x28, 0x09, 0x52, 0x08, 0x75,
	0x6e, 0x69, 0x71, 0x75, 0x65, 0x49, 0x64, 0x12, 0x4a, 0x0a, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x18, 0x0a, 0x20, 0x03, 0x28, 0x0b, 0x32, 0x2e, 0x2e, 0x6b, 0x61, 0x79, 0x74,
	0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x7a,
	0x75, 0x72, 0x65, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x2e, 0x4d, 0x65, 0x74, 0x61,
	0x64, 0x61, 0x74, 0x61, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x08, 0x6d, 0x65, 0x74, 0x61, 0x64,
	0x61, 0x74, 0x61, 0x12, 0x3e, 0x0a, 0x04, 0x74, 0x61, 0x67, 0x73, 0x18, 0x0b, 0x20, 0x03, 0x28,
	0x0b, 0x32, 0x2a, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69,
	0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x7a, 0x75, 0x72, 0x65, 0x52, 0x65, 0x73, 0x6f, 0x75,
	0x72, 0x63, 0x65, 0x2e, 0x54, 0x61, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x52, 0x04, 0x74,
	0x61, 0x67, 0x73, 0x1a, 0x3b, 0x0a, 0x0d, 0x4d, 0x65, 0x74, 0x61, 0x64, 0x61, 0x74, 0x61, 0x45,
	0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a, 0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28,
	0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12, 0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18,
	0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01,
	0x1a, 0x37, 0x0a, 0x09, 0x54, 0x61, 0x67, 0x73, 0x45, 0x6e, 0x74, 0x72, 0x79, 0x12, 0x10, 0x0a,
	0x03, 0x6b, 0x65, 0x79, 0x18, 0x01, 0x20, 0x01, 0x28, 0x09, 0x52, 0x03, 0x6b, 0x65, 0x79, 0x12,
	0x14, 0x0a, 0x05, 0x76, 0x61, 0x6c, 0x75, 0x65, 0x18, 0x02, 0x20, 0x01, 0x28, 0x09, 0x52, 0x05,
	0x76, 0x61, 0x6c, 0x75, 0x65, 0x3a, 0x02, 0x38, 0x01, 0x22, 0xf8, 0x01, 0x0a, 0x14, 0x44, 0x65,
	0x6c, 0x69, 0x76, 0x65, 0x72, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65,
	0x73, 0x74, 0x12, 0x15, 0x0a, 0x06, 0x6a, 0x6f, 0x62, 0x5f, 0x69, 0x64, 0x18, 0x01, 0x20, 0x01,
	0x28, 0x0d, 0x52, 0x05, 0x6a, 0x6f, 0x62, 0x49, 0x64, 0x12, 0x22, 0x0a, 0x0d, 0x70, 0x61, 0x72,
	0x65, 0x6e, 0x74, 0x5f, 0x6a, 0x6f, 0x62, 0x5f, 0x69, 0x64, 0x18, 0x02, 0x20, 0x01, 0x28, 0x0d,
	0x52, 0x0b, 0x70, 0x61, 0x72, 0x65, 0x6e, 0x74, 0x4a, 0x6f, 0x62, 0x49, 0x64, 0x12, 0x16, 0x0a,
	0x06, 0x73, 0x74, 0x61, 0x74, 0x75, 0x73, 0x18, 0x03, 0x20, 0x01, 0x28, 0x09, 0x52, 0x06, 0x73,
	0x74, 0x61, 0x74, 0x75, 0x73, 0x12, 0x14, 0x0a, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x18, 0x04,
	0x20, 0x01, 0x28, 0x09, 0x52, 0x05, 0x65, 0x72, 0x72, 0x6f, 0x72, 0x12, 0x41, 0x0a, 0x0c, 0x64,
	0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x5f, 0x6a, 0x6f, 0x62, 0x18, 0x05, 0x20, 0x01, 0x28,
	0x0b, 0x32, 0x1e, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69,
	0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x4a, 0x6f,
	0x62, 0x52, 0x0b, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x4a, 0x6f, 0x62, 0x12, 0x34,
	0x0a, 0x16, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x64, 0x5f, 0x72, 0x65, 0x73, 0x6f,
	0x75, 0x72, 0x63, 0x65, 0x5f, 0x69, 0x64, 0x73, 0x18, 0x06, 0x20, 0x03, 0x28, 0x09, 0x52, 0x14,
	0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x64, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63,
	0x65, 0x49, 0x64, 0x73, 0x22, 0x0c, 0x0a, 0x0a, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65,
	0x4f, 0x4b, 0x32, 0xa2, 0x02, 0x0a, 0x0f, 0x44, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x53,
	0x65, 0x72, 0x76, 0x69, 0x63, 0x65, 0x12, 0x59, 0x0a, 0x0d, 0x44, 0x65, 0x6c, 0x69, 0x76, 0x65,
	0x72, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x12, 0x27, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e,
	0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x44, 0x65, 0x6c, 0x69,
	0x76, 0x65, 0x72, 0x52, 0x65, 0x73, 0x75, 0x6c, 0x74, 0x52, 0x65, 0x71, 0x75, 0x65, 0x73, 0x74,
	0x1a, 0x1d, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62,
	0x65, 0x2e, 0x76, 0x31, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x4f, 0x4b, 0x22,
	0x00, 0x12, 0x57, 0x0a, 0x13, 0x44, 0x65, 0x6c, 0x69, 0x76, 0x65, 0x72, 0x41, 0x57, 0x53, 0x52,
	0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x12, 0x1f, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75,
	0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x57, 0x53,
	0x52, 0x65, 0x73, 0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x1a, 0x1d, 0x2e, 0x6b, 0x61, 0x79, 0x74,
	0x75, 0x2e, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x52, 0x65,
	0x73, 0x70, 0x6f, 0x6e, 0x73, 0x65, 0x4f, 0x4b, 0x22, 0x00, 0x12, 0x5b, 0x0a, 0x15, 0x44, 0x65,
	0x6c, 0x69, 0x76, 0x65, 0x72, 0x41, 0x7a, 0x75, 0x72, 0x65, 0x52, 0x65, 0x73, 0x6f, 0x75, 0x72,
	0x63, 0x65, 0x73, 0x12, 0x21, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64, 0x65, 0x73, 0x63,
	0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x41, 0x7a, 0x75, 0x72, 0x65, 0x52, 0x65, 0x73,
	0x6f, 0x75, 0x72, 0x63, 0x65, 0x73, 0x1a, 0x1d, 0x2e, 0x6b, 0x61, 0x79, 0x74, 0x75, 0x2e, 0x64,
	0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2e, 0x76, 0x31, 0x2e, 0x52, 0x65, 0x73, 0x70, 0x6f,
	0x6e, 0x73, 0x65, 0x4f, 0x4b, 0x22, 0x00, 0x42, 0x43, 0x5a, 0x41, 0x67, 0x69, 0x74, 0x6c, 0x61,
	0x62, 0x2e, 0x63, 0x6f, 0x6d, 0x2f, 0x6b, 0x65, 0x69, 0x62, 0x69, 0x65, 0x6e, 0x67, 0x69, 0x6e,
	0x65, 0x2f, 0x6b, 0x65, 0x69, 0x62, 0x69, 0x2d, 0x65, 0x6e, 0x67, 0x69, 0x6e, 0x65, 0x2f, 0x70,
	0x6b, 0x67, 0x2f, 0x64, 0x65, 0x73, 0x63, 0x72, 0x69, 0x62, 0x65, 0x2f, 0x70, 0x72, 0x6f, 0x74,
	0x6f, 0x2f, 0x73, 0x72, 0x63, 0x2f, 0x67, 0x6f, 0x6c, 0x61, 0x6e, 0x67, 0x62, 0x06, 0x70, 0x72,
	0x6f, 0x74, 0x6f, 0x33,
}

var (
	file_pkg_describe_proto_describe_proto_rawDescOnce sync.Once
	file_pkg_describe_proto_describe_proto_rawDescData = file_pkg_describe_proto_describe_proto_rawDesc
)

func file_pkg_describe_proto_describe_proto_rawDescGZIP() []byte {
	file_pkg_describe_proto_describe_proto_rawDescOnce.Do(func() {
		file_pkg_describe_proto_describe_proto_rawDescData = protoimpl.X.CompressGZIP(file_pkg_describe_proto_describe_proto_rawDescData)
	})
	return file_pkg_describe_proto_describe_proto_rawDescData
}

var file_pkg_describe_proto_describe_proto_msgTypes = make([]protoimpl.MessageInfo, 11)
var file_pkg_describe_proto_describe_proto_goTypes = []interface{}{
	(*DescribeJob)(nil),          // 0: kaytu.describe.v1.DescribeJob
	(*AWSResources)(nil),         // 1: kaytu.describe.v1.AWSResources
	(*AWSResource)(nil),          // 2: kaytu.describe.v1.AWSResource
	(*AzureResources)(nil),       // 3: kaytu.describe.v1.AzureResources
	(*AzureResource)(nil),        // 4: kaytu.describe.v1.AzureResource
	(*DeliverResultRequest)(nil), // 5: kaytu.describe.v1.DeliverResultRequest
	(*ResponseOK)(nil),           // 6: kaytu.describe.v1.ResponseOK
	nil,                          // 7: kaytu.describe.v1.AWSResource.MetadataEntry
	nil,                          // 8: kaytu.describe.v1.AWSResource.TagsEntry
	nil,                          // 9: kaytu.describe.v1.AzureResource.MetadataEntry
	nil,                          // 10: kaytu.describe.v1.AzureResource.TagsEntry
}
var file_pkg_describe_proto_describe_proto_depIdxs = []int32{
	2,  // 0: kaytu.describe.v1.AWSResources.resources:type_name -> kaytu.describe.v1.AWSResource
	0,  // 1: kaytu.describe.v1.AWSResource.job:type_name -> kaytu.describe.v1.DescribeJob
	7,  // 2: kaytu.describe.v1.AWSResource.metadata:type_name -> kaytu.describe.v1.AWSResource.MetadataEntry
	8,  // 3: kaytu.describe.v1.AWSResource.tags:type_name -> kaytu.describe.v1.AWSResource.TagsEntry
	4,  // 4: kaytu.describe.v1.AzureResources.resources:type_name -> kaytu.describe.v1.AzureResource
	0,  // 5: kaytu.describe.v1.AzureResource.job:type_name -> kaytu.describe.v1.DescribeJob
	9,  // 6: kaytu.describe.v1.AzureResource.metadata:type_name -> kaytu.describe.v1.AzureResource.MetadataEntry
	10, // 7: kaytu.describe.v1.AzureResource.tags:type_name -> kaytu.describe.v1.AzureResource.TagsEntry
	0,  // 8: kaytu.describe.v1.DeliverResultRequest.describe_job:type_name -> kaytu.describe.v1.DescribeJob
	5,  // 9: kaytu.describe.v1.DescribeService.DeliverResult:input_type -> kaytu.describe.v1.DeliverResultRequest
	1,  // 10: kaytu.describe.v1.DescribeService.DeliverAWSResources:input_type -> kaytu.describe.v1.AWSResources
	3,  // 11: kaytu.describe.v1.DescribeService.DeliverAzureResources:input_type -> kaytu.describe.v1.AzureResources
	6,  // 12: kaytu.describe.v1.DescribeService.DeliverResult:output_type -> kaytu.describe.v1.ResponseOK
	6,  // 13: kaytu.describe.v1.DescribeService.DeliverAWSResources:output_type -> kaytu.describe.v1.ResponseOK
	6,  // 14: kaytu.describe.v1.DescribeService.DeliverAzureResources:output_type -> kaytu.describe.v1.ResponseOK
	12, // [12:15] is the sub-list for method output_type
	9,  // [9:12] is the sub-list for method input_type
	9,  // [9:9] is the sub-list for extension type_name
	9,  // [9:9] is the sub-list for extension extendee
	0,  // [0:9] is the sub-list for field type_name
}

func init() { file_pkg_describe_proto_describe_proto_init() }
func file_pkg_describe_proto_describe_proto_init() {
	if File_pkg_describe_proto_describe_proto != nil {
		return
	}
	if !protoimpl.UnsafeEnabled {
		file_pkg_describe_proto_describe_proto_msgTypes[0].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DescribeJob); i {
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
		file_pkg_describe_proto_describe_proto_msgTypes[1].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AWSResources); i {
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
		file_pkg_describe_proto_describe_proto_msgTypes[2].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AWSResource); i {
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
		file_pkg_describe_proto_describe_proto_msgTypes[3].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AzureResources); i {
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
		file_pkg_describe_proto_describe_proto_msgTypes[4].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*AzureResource); i {
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
		file_pkg_describe_proto_describe_proto_msgTypes[5].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*DeliverResultRequest); i {
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
		file_pkg_describe_proto_describe_proto_msgTypes[6].Exporter = func(v interface{}, i int) interface{} {
			switch v := v.(*ResponseOK); i {
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
			RawDescriptor: file_pkg_describe_proto_describe_proto_rawDesc,
			NumEnums:      0,
			NumMessages:   11,
			NumExtensions: 0,
			NumServices:   1,
		},
		GoTypes:           file_pkg_describe_proto_describe_proto_goTypes,
		DependencyIndexes: file_pkg_describe_proto_describe_proto_depIdxs,
		MessageInfos:      file_pkg_describe_proto_describe_proto_msgTypes,
	}.Build()
	File_pkg_describe_proto_describe_proto = out.File
	file_pkg_describe_proto_describe_proto_rawDesc = nil
	file_pkg_describe_proto_describe_proto_goTypes = nil
	file_pkg_describe_proto_describe_proto_depIdxs = nil
}
