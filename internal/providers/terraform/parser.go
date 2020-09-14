package terraform

import (
	"fmt"
	"regexp"
	"strconv"
	"strings"

	"github.com/infracost/infracost/pkg/schema"

	"github.com/tidwall/gjson"
)

// These show differently in the plan JSON for Terraform 0.12 and 0.13
var infracostProviderNames = []string{"infracost", "infracost.io/infracost/infracost"}

func createResource(r *schema.ResourceData, u *schema.ResourceData) *schema.Resource {
	registry := getResourceRegistry()

	if rFunc, ok := (*registry)[r.Type]; ok {
		return rFunc(r, u)
	}

	return nil
}

func parsePlanJSON(j []byte) []*schema.Resource {
	p := gjson.ParseBytes(j)
	providerConf := p.Get("configuration.provider_config")
	planVals := p.Get("planned_values.root_module")
	conf := p.Get("configuration.root_module")

	resources := make([]*schema.Resource, 0)

	resData := parseResourceData(p, providerConf, planVals)
	parseReferences(resData, conf)
	resUsage := buildUsageResourceDataMap(resData)
	resData = stripInfracostResources(resData)

	for _, r := range resData {
		if res := createResource(r, resUsage[r.Address]); res != nil {
			resources = append(resources, res)
		}
	}

	return resources
}

func parseResourceData(plan, provider, planVals gjson.Result) map[string]*schema.ResourceData {
	defaultRegion := parseAwsRegion(provider)

	resources := make(map[string]*schema.ResourceData)

	for _, r := range planVals.Get("resources").Array() {
		t := r.Get("type").String()
		provider := r.Get("provider_name").String()
		addr := r.Get("address").String()
		v := r.Get("values")

		// Override the region with the region from the arn if exists
		region := defaultRegion
		if v.Get("arn").Exists() {
			region = strings.Split(v.Get("arn").String(), ":")[3]
		}
		v = schema.AddRawValue(v, "region", region)

		resources[addr] = schema.NewResourceData(t, provider, addr, v)
	}

	// Recursively add any resources for child modules
	for _, m := range planVals.Get("child_modules").Array() {
		for addr, d := range parseResourceData(plan, provider, m) {
			resources[addr] = d
		}
	}
	return resources
}

func parseAwsRegion(providerConfig gjson.Result) string {
	// Find region from terraform provider config
	region := providerConfig.Get("aws.expressions.region.constant_value").String()
	if region == "" {
		region = "us-east-1"
	}

	return region
}

func buildUsageResourceDataMap(resData map[string]*schema.ResourceData) map[string]*schema.ResourceData {
	u := make(map[string]*schema.ResourceData)

	for _, r := range resData {
		if isInfracostResource(r) {
			for _, ref := range r.References("resources") {
				u[ref.Address] = r
			}
		}
	}

	return u
}

func stripInfracostResources(resData map[string]*schema.ResourceData) map[string]*schema.ResourceData {
	n := make(map[string]*schema.ResourceData)

	for addr, d := range resData {
		if !isInfracostResource(d) {
			n[addr] = d
		}
	}

	return n
}

func parseReferences(resData map[string]*schema.ResourceData, conf gjson.Result) {
	for addr, res := range resData {
		resConf := getConfigurationJSONForResourceAddress(conf, addr)

		var refsMap = make(map[string][]string)
		for attr, j := range resConf.Get("expressions").Map() {
			getReferences(res, attr, j, &refsMap)
		}

		for attr, refs := range refsMap {
			for _, ref := range refs {
				if ref == "count.index" {
					continue
				}

				var refAddr string
				if containsString(refs, "count.index") {
					refAddr = fmt.Sprintf("%s%s[%d]", addressModulePart(addr), ref, addressCountIndex(addr))
				} else {
					refAddr = fmt.Sprintf("%s%s", addressModulePart(addr), ref)
				}

				if refData, ok := resData[refAddr]; ok {
					res.AddReference(attr, refData)
				}
			}
		}
	}
}

func getReferences(resData *schema.ResourceData, attr string, j gjson.Result, refs *map[string][]string) {
	if j.Get("references").Exists() {
		for _, ref := range j.Get("references").Array() {
			if _, ok := (*refs)[attr]; !ok {
				(*refs)[attr] = make([]string, 0, 1)
			}

			(*refs)[attr] = append((*refs)[attr], ref.String())
		}
	} else if j.IsArray() {
		for i, attributeJSONItem := range j.Array() {
			getReferences(resData, fmt.Sprintf("%s.%d", attr, i), attributeJSONItem, refs)
		}
	} else if j.Type.String() == "JSON" {
		j.ForEach(func(childAttribute gjson.Result, childAttributeJSON gjson.Result) bool {
			getReferences(resData, fmt.Sprintf("%s.%s", attr, childAttribute), childAttributeJSON, refs)

			return true
		})
	}
}

func getConfigurationJSONForResourceAddress(conf gjson.Result, addr string) gjson.Result {
	c := getConfigurationJSONForModulePath(conf, getModuleNames(addr))

	return c.Get(fmt.Sprintf(`resources.#(address="%s")`, removeAddressArrayPart(addressResourcePart(addr))))
}

func getConfigurationJSONForModulePath(conf gjson.Result, names []string) gjson.Result {
	if len(names) == 0 {
		return conf
	}

	// Build up the gjson search key
	p := make([]string, 0, len(names))
	for _, n := range names {
		p = append(p, fmt.Sprintf("module_calls.%s.module", n))
	}

	return conf.Get(strings.Join(p, "."))
}

func isInfracostResource(res *schema.ResourceData) bool {
	for _, p := range infracostProviderNames {
		if res.ProviderName == p {
			return true
		}
	}

	return false
}

// addressResourcePart parses a resource addr and returns resource suffix (without the module prefix).
// For example: `module.name1.module.name2.resource` will return `name2.resource`
func addressResourcePart(addr string) string {
	p := strings.Split(addr, ".")

	if len(p) >= 3 && p[len(p)-3] == "data" {
		return strings.Join(p[len(p)-3:], ".")
	}

	return strings.Join(p[len(p)-2:], ".")
}

// addressModulePart parses a resource addr and returns module prefix.
// For example: `module.name1.module.name2.resource` will return `module.name1.module.name2.`
func addressModulePart(addr string) string {
	ap := strings.Split(addr, ".")
	var mp []string

	if len(ap) >= 3 && ap[len(ap)-3] == "data" {
		mp = ap[:len(ap)-3]
	} else {
		mp = ap[:len(ap)-2]
	}

	if len(mp) == 0 {
		return ""
	}

	return fmt.Sprintf("%s.", strings.Join(mp, "."))
}

func getModuleNames(addr string) []string {
	r := regexp.MustCompile(`module\.([^\[]*)`)
	matches := r.FindAllStringSubmatch(addressModulePart(addr), -1)
	if matches == nil {
		return []string{}
	}

	n := make([]string, 0, len(matches))
	for _, m := range matches {
		n = append(n, m[1])
	}

	return n
}

func addressCountIndex(addr string) int {
	r := regexp.MustCompile(`\[(\d+)\]`)
	m := r.FindStringSubmatch(addr)

	if len(m) > 0 {
		i, _ := strconv.Atoi(m[1]) // TODO: unhandled error

		return i
	}

	return -1
}

func removeAddressArrayPart(addr string) string {
	r := regexp.MustCompile(`([^\[]+)`)
	m := r.FindStringSubmatch(addressResourcePart(addr))

	return m[1]
}

func containsString(a []string, s string) bool {
	for _, i := range a {
		if i == s {
			return true
		}
	}

	return false
}