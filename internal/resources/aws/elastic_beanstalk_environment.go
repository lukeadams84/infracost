package aws

import (
	"github.com/infracost/infracost/internal/resources"
	"github.com/infracost/infracost/internal/schema"
)

// ElasticBeanstalkEnvironment struct represents <TODO: cloud service short description>.
//
// <TODO: Add any important information about the resource and links to the
// pricing pages or documentation that might be useful to developers in the future, e.g:>
//
// Resource information: https://aws.amazon.com/elasticbeanstalk/
// Pricing information: https://aws.amazon.com/elasticbeanstalk/pricing/
type ElasticBeanstalkEnvironment struct {
	Address string
	Region  string
	Name    string

	InstanceCount    *int64
	InstanceType     string
	RDSIncluded      bool
	LoadBalancerType string
	StreamLogs       bool

	RootBlockDevice     *EBSVolume
	CloudwatchLogGroup  *CloudwatchLogGroup
	LoadBalancer        *LB
	ElasticLoadBalancer *ELB
	DBInstance          *DBInstance
	LaunchConfiguration *LaunchConfiguration
}

// ElasticBeanstalkEnvironmentUsageSchema defines a list which represents the usage schema of ElasticBeanstalkEnvironment.
var ElasticBeanstalkEnvironmentUsageSchema = []*schema.UsageItem{

	{
		Key:          "cloudwatch",
		DefaultValue: CloudwatchLogGroupUsageSchema,
		ValueType:    schema.SubResourceUsage,
	},
	{
		Key:          "lb",
		DefaultValue: LBUsageSchema,
		ValueType:    schema.SubResourceUsage,
	},
	{
		Key:          "elb",
		DefaultValue: ELBUsageSchema,
		ValueType:    schema.SubResourceUsage,
	},
	{
		Key:          "db",
		DefaultValue: DBInstanceUsageSchema,
		ValueType:    schema.SubResourceUsage,
	},
	{
		Key:          "ec2",
		DefaultValue: LaunchConfigurationUsageSchema,
		ValueType:    schema.SubResourceUsage,
	},
}

// PopulateUsage parses the u schema.UsageData into the ElasticBeanstalkEnvironment.
// It uses the `infracost_usage` struct tags to populate data into the ElasticBeanstalkEnvironment.
func (r *ElasticBeanstalkEnvironment) PopulateUsage(u *schema.UsageData) {

	if u == nil {
		return
	}

	if r.ElasticLoadBalancer != nil {
		resources.PopulateArgsWithUsage(r.ElasticLoadBalancer, schema.NewUsageData("elb", u.Get("elb").Map()))
	}
	if r.LoadBalancer != nil {
		resources.PopulateArgsWithUsage(r.LoadBalancer, schema.NewUsageData("lb", u.Get("lb").Map()))
	}
	if r.DBInstance != nil {
		resources.PopulateArgsWithUsage(r.DBInstance, schema.NewUsageData("db", u.Get("db").Map()))
	}
	if r.CloudwatchLogGroup != nil {
		resources.PopulateArgsWithUsage(r.CloudwatchLogGroup, schema.NewUsageData("cloudwatch", u.Get("cloudwatch").Map()))
	}
	resources.PopulateArgsWithUsage(r.LaunchConfiguration, schema.NewUsageData("ec2", u.Get("ec2").Map()))
}

// BuildResource builds a schema.Resource from a valid ElasticBeanstalkEnvironment struct.
// This method is called after the resource is initialised by an IaC provider.
// See providers folder for more information.
func (r *ElasticBeanstalkEnvironment) BuildResource() *schema.Resource {

	a := &schema.Resource{
		Name:        r.Address,
		UsageSchema: ElasticBeanstalkEnvironmentUsageSchema,
	}

	a.SubResources = append(a.SubResources, r.LaunchConfiguration.BuildResource())

	if r.DBInstance != nil {
		a.SubResources = append(a.SubResources, r.DBInstance.BuildResource())

	}
	if r.CloudwatchLogGroup != nil {
		a.SubResources = append(a.SubResources, r.CloudwatchLogGroup.BuildResource())
	}
	switch r.LoadBalancerType {
	case "classic":
		a.SubResources = append(a.SubResources, r.ElasticLoadBalancer.BuildResource())
	default:
		a.SubResources = append(a.SubResources, r.LoadBalancer.BuildResource())
	}

	return a

}
