package aws

import (
	"github.com/infracost/infracost/internal/resources/aws"
	"github.com/infracost/infracost/internal/schema"
)

func getDBInstanceRegistryItem() *schema.RegistryItem {
	return &schema.RegistryItem{
		Name:  "aws_db_instance",
		RFunc: NewDBInstance,
	}
}

func NewDBInstance(d *schema.ResourceData, u *schema.UsageData) *schema.Resource {
	piEnabled := d.Get("performance_insights_enabled").Bool()
	piLongTerm := piEnabled && d.Get("performance_insights_retention_period").Int() > 7

	r := &aws.DBInstance{
		Address:                              d.Address,
		Region:                               d.Get("region").String(),
		InstanceClass:                        d.Get("instance_class").String(),
		Engine:                               d.Get("engine").String(),
		MultiAZ:                              d.Get("multi_az").Bool(),
		LicenseModel:                         d.Get("license_model").String(),
		BackupRetentionPeriod:                d.Get("backup_retention_period").Int(),
		IOPS:                                 d.Get("iops").Float(),
		StorageType:                          d.Get("storage_type").String(),
		PerformanceInsightsEnabled:           piEnabled,
		PerformanceInsightsLongTermRetention: piLongTerm,
	}

	if !d.IsEmpty("allocated_storage") {
		r.AllocatedStorageGB = floatPtr(d.Get("allocated_storage").Float())
	}

	r.PopulateUsage(u)
	return r.BuildResource()
}
