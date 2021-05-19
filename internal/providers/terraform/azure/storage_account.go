package azure

import (
	"fmt"

	"github.com/infracost/infracost/internal/schema"
	"github.com/infracost/infracost/internal/usage"
	"github.com/shopspring/decimal"
	log "github.com/sirupsen/logrus"
	"github.com/tidwall/gjson"
)

func GetAzureStorageAccountRegistryItem() *schema.RegistryItem {
	return &schema.RegistryItem{
		Name:  "azurerm_storage_account",
		RFunc: NewAzureStorageAccount,
	}
}

func NewAzureStorageAccount(d *schema.ResourceData, u *schema.UsageData) *schema.Resource {
	var costComponents []*schema.CostComponent

	location := d.Get("location").String()
	accountKind := "StorageV2"
	if d.Get("account_kind").Type != gjson.Null {
		accountKind = d.Get("account_kind").String()
	}

	if accountKind != "BlockBlobStorage" {
		log.Warnf("Skipping resource %s. Infracost only supports BlockBlobStorage account kind", d.Address)
		return nil
	}

	accountReplicationType := d.Get("account_replication_type").String()
	accountTier := d.Get("account_tier").String()
	accessTier := "Hot"
	if d.Get("access_tier").Type != gjson.Null {
		accessTier = d.Get("access_tier").String()
	}

	productName := map[string]string{
		"Standard": "Blob Storage",
		"Premium":  "Premium Block Blob",
	}[accountTier]

	if productName == "" {
		log.Warnf("Unrecognized account tier for resource %s: %s", d.Address, accountTier)
		return nil
	}

	validPremiumReplicationTypes := []string{"ZRS", "LRS"}
	validStandardReplicationTypes := []string{"LRS", "GRS", "RAGRS"}

	if accessTier == "Premium" && (!Contains(validPremiumReplicationTypes, accountReplicationType) || !Contains(validStandardReplicationTypes, accountReplicationType)) {
		log.Warnf("%s redundancy does not supports for %s performance tier", accountReplicationType, accountTier)
	}

	var capacity, writeOperations, listOperations, readOperations, otherOperations, dataRetrieval, dataWrite, blobIndex *decimal.Decimal

	if accountReplicationType == "RAGRS" {
		accountReplicationType = "RA-GRS"
	}

	skuName := fmt.Sprintf("%s %s", accessTier, accountReplicationType)
	if accountTier == "Premium" {
		skuName = fmt.Sprintf("%s %s", accountTier, accountReplicationType)
	}

	if u != nil && u.Get("storage_gb").Exists() {
		capacity = decimalPtr(decimal.NewFromInt(u.Get("storage_gb").Int()))

		if accessTier == "Hot" {
			dataStorageTiers := []int{51200, 512000}
			dataStorageQuantities := usage.CalculateTierBuckets(*capacity, dataStorageTiers)

			costComponents = append(costComponents, blobDataStorageCostComponent(
				location,
				"Capacity (first 50TB)",
				skuName,
				"0",
				productName,
				&dataStorageQuantities[0]))

			if dataStorageQuantities[1].GreaterThan(decimal.Zero) {
				costComponents = append(costComponents, blobDataStorageCostComponent(
					location,
					"Capacity (next 450TB)",
					skuName,
					"51200",
					productName,
					&dataStorageQuantities[1]))
			}

			if dataStorageQuantities[2].GreaterThan(decimal.Zero) {
				costComponents = append(costComponents, blobDataStorageCostComponent(
					location,
					"Capacity (over 500TB)",
					skuName,
					"512000",
					productName,
					&dataStorageQuantities[2]))
			}
		} else {
			costComponents = append(costComponents, blobDataStorageCostComponent(location, "Capacity", skuName, "0", productName, capacity))
		}
	} else {
		var unknown *decimal.Decimal

		costComponents = append(costComponents, blobDataStorageCostComponent(location, "Capacity", skuName, "0", productName, unknown))
	}

	if u != nil && u.Get("monthly_write_operations").Exists() {
		writeOperations = decimalPtr(decimal.NewFromInt(u.Get("monthly_write_operations").Int()))
	}
	costComponents = append(costComponents, blobOperationsCostComponent(
		location,
		"Write operations",
		"10K operations",
		skuName,
		"/Write Operations$/",
		productName,
		writeOperations,
		10000))

	lccoSkuName := skuName
	if u != nil && u.Get("monthly_list_and_create_container_operations").Exists() {
		listOperations = decimalPtr(decimal.NewFromInt(u.Get("monthly_list_and_create_container_operations").Int()))

		if skuName == "Hot RA-GRS" {
			lccoSkuName = "Hot GRS"
		}
	}
	costComponents = append(costComponents, blobOperationsCostComponent(
		location,
		"List and create container operations",
		"10K operations",
		lccoSkuName,
		"/List and Create Container Operations$/",
		productName,
		listOperations,
		10000))

	if u != nil && u.Get("monthly_read_operations").Exists() {
		readOperations = decimalPtr(decimal.NewFromInt(u.Get("monthly_read_operations").Int()))
	}
	costComponents = append(costComponents, blobOperationsCostComponent(
		location,
		"Read operations",
		"10K operations",
		skuName,
		"/Read Operations$/",
		productName,
		readOperations,
		10000))

	if u != nil && u.Get("monthly_other_operations").Exists() {
		otherOperations = decimalPtr(decimal.NewFromInt(u.Get("monthly_other_operations").Int()))
	}
	costComponents = append(costComponents, blobOperationsCostComponent(
		location,
		"All other operations",
		"10K operations",
		skuName,
		"/All Other Operations$/",
		productName,
		otherOperations,
		10000))

	if accountTier != "Premium" {
		if u != nil && u.Get("monthly_data_retrieval_gb").Exists() {
			dataRetrieval = decimalPtr(decimal.NewFromInt(u.Get("monthly_data_retrieval_gb").Int()))
		}
		costComponents = append(costComponents, blobOperationsCostComponent(
			location,
			"Data retrieval",
			"GB",
			skuName,
			"/Data Retrieval$/",
			productName,
			dataRetrieval,
			1))

		if u != nil && u.Get("monthly_data_write_gb").Exists() {
			dataWrite = decimalPtr(decimal.NewFromInt(u.Get("monthly_data_write_gb").Int()))
		}
		costComponents = append(costComponents, blobOperationsCostComponent(
			location,
			"Data write",
			"GB",
			skuName,
			"/Data Write$/",
			productName,
			dataWrite,
			1))

		if u != nil && u.Get("blob_index_tags").Exists() {
			blobIndex = decimalPtr(decimal.NewFromInt(u.Get("blob_index_tags").Int()))
		}
		costComponents = append(costComponents, blobOperationsCostComponent(
			location,
			"Blob index",
			"10K tags",
			skuName,
			"/Index Tags$/",
			productName,
			blobIndex,
			10000))
	}

	return &schema.Resource{
		Name:           d.Address,
		CostComponents: costComponents,
	}
}

func blobDataStorageCostComponent(location, name, skuName, startUsage, productName string, quantity *decimal.Decimal) *schema.CostComponent {
	return &schema.CostComponent{
		Name:                 name,
		Unit:                 "GB",
		UnitMultiplier:       1,
		MonthlyQuantity:      quantity,
		IgnoreIfMissingPrice: true,
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr("azure"),
			Region:        strPtr(location),
			Service:       strPtr("Storage"),
			ProductFamily: strPtr("Storage"),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", Value: strPtr(productName)},
				{Key: "skuName", Value: strPtr(skuName)},
				{Key: "meterName", ValueRegex: strPtr("/Data Stored$/")},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption:   strPtr("Consumption"),
			StartUsageAmount: strPtr(startUsage),
		},
	}
}

func blobOperationsCostComponent(location, name, unit, skuName, meterName, productName string, quantity *decimal.Decimal, multi int) *schema.CostComponent {
	if quantity != nil {
		quantity = decimalPtr(quantity.Div(decimal.NewFromInt(int64(multi))))
	}

	return &schema.CostComponent{
		Name:                 name,
		Unit:                 unit,
		UnitMultiplier:       1,
		MonthlyQuantity:      quantity,
		IgnoreIfMissingPrice: true,
		ProductFilter: &schema.ProductFilter{
			VendorName:    strPtr("azure"),
			Region:        strPtr(location),
			Service:       strPtr("Storage"),
			ProductFamily: strPtr("Storage"),
			AttributeFilters: []*schema.AttributeFilter{
				{Key: "productName", Value: strPtr(productName)},
				{Key: "skuName", Value: strPtr(skuName)},
				{Key: "meterName", ValueRegex: strPtr(meterName)},
			},
		},
		PriceFilter: &schema.PriceFilter{
			PurchaseOption: strPtr("Consumption"),
		},
	}
}