package exoscale

import (
	"context"
	"errors"
	"fmt"
	"net"
	"os"
	"reflect"
	"strconv"
	"strings"
	"testing"

	egoscale "github.com/exoscale/egoscale/v2"
	exoapi "github.com/exoscale/egoscale/v2/api"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/acctest"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/resource"
	"github.com/hashicorp/terraform-plugin-sdk/v2/helper/schema"
	"github.com/hashicorp/terraform-plugin-sdk/v2/terraform"
	"github.com/stretchr/testify/require"
)

var (
	testAccResourceSecurityGroupRulesTestSecurityGroupName       = acctest.RandomWithPrefix(testPrefix)
	testAccResourceSecurityGroupRulesICMPCode0             int64 = 0
	testAccResourceSecurityGroupRulesICMPType8             int64 = 8
	testAccResourceSecurityGroupRulesICMPv6Type128         int64 = 128

	testAccResourceSecurityGroupRulesConfigCreate = fmt.Sprintf(`
data "exoscale_security_group" "default" {
  name = "default"
}

resource "exoscale_security_group" "test" {
  name = "%s"
}

resource "exoscale_security_group_rules" "rules" {
  security_group_id = exoscale_security_group.test.id

  ingress {
    protocol = "ICMP"
    icmp_type = 8
    icmp_code = 0
    cidr_list = ["0.0.0.0/0"]
  }

  ingress {
    protocol = "ICMPv6"
    icmp_type = 128
    icmp_code = 0
    cidr_list = ["::/0"]
  }

  ingress {
    protocol = "TCP"
    cidr_list = ["10.0.0.0/24", "::/0"]
    ports = ["22", "8000-8888"]
    user_security_group_list = [exoscale_security_group.test.name, data.exoscale_security_group.default.name]
  }

  ingress {
	protocol = "ESP"
	cidr_list = ["192.168.0.0/24", "::/0"]
	user_security_group_list = [data.exoscale_security_group.default.name]
  }

  egress {
    protocol = "UDP"
    cidr_list = ["192.168.0.0/24", "::/0"]
    ports = ["44", "2375-2377"]
    user_security_group_list = [data.exoscale_security_group.default.name]
  }
}
`,
		testAccResourceSecurityGroupRulesTestSecurityGroupName,
	)

	testAccResourceSecurityGroupRulesConfigUpdate = fmt.Sprintf(`
data "exoscale_security_group" "default" {
  name = "default"
}

resource "exoscale_security_group" "test" {
  name = "%s"
}

resource "exoscale_security_group_rules" "rules" {
  security_group_id = exoscale_security_group.test.id

  ingress {
    protocol = "ICMP"
    icmp_type = 8
    icmp_code = 0
    cidr_list = ["0.0.0.0/0"]
  }

  ingress {
    protocol = "ICMPv6"
    icmp_type = 128
    icmp_code = 0
    cidr_list = ["::/0"]
  }

  ingress {
    protocol = "TCP"
    cidr_list = ["10.0.0.0/24", "::/0"]
    ports = ["2222", "8000-8888"]
    user_security_group_list = [exoscale_security_group.test.name, data.exoscale_security_group.default.name]
  }

  ingress {
	protocol = "ESP"
	cidr_list = ["192.168.0.0/24", "::/0"]
	user_security_group_list = [exoscale_security_group.test.name]
  }

  egress {
    protocol = "UDP"
    cidr_list = ["192.168.0.0/24", "::/0"]
    ports = ["4444", "2375-2377"]
    user_security_group_list = [data.exoscale_security_group.default.name]
  }
}
`,
		testAccResourceSecurityGroupRulesTestSecurityGroupName,
	)
)

func TestPreparePorts(t *testing.T) {
	ports := preparePorts(schema.NewSet(schema.HashString, []interface{}{"22", "10-20"}))

	for _, portRange := range ports {
		if portRange[0] == 22 && portRange[1] != 22 {
			t.Errorf("bad port, wanted 22-22, got %#v", portRange)
		}

		if portRange[0] == 10 && portRange[1] != 20 {
			t.Errorf("bad port, wanted 10-20, got %#v", ports[1])
		}
	}
}

func TestAccResourceSecurityGroupRules(t *testing.T) {
	testSecurityGroup := new(egoscale.SecurityGroup)
	defaultSecurityGroup := new(egoscale.SecurityGroup)
	mustParseCIDR := func(t *testing.T, cidr string) *net.IPNet {
		_, cidrp, err := net.ParseCIDR(cidr)
		if err != nil {
			t.Fatalf("unable to parse CIDR %q: %s", cidr, err)
		}
		return cidrp
	}
	portValPtr := func(p int) *uint16 {
		portVal := uint16(p)
		return &portVal
	}

	resource.Test(t, resource.TestCase{
		PreCheck: func() {
			testAccPreCheck(t)

			// Retrieve the current organization's default Security Group.
			client, err := egoscale.NewClient(
				os.Getenv("EXOSCALE_API_KEY"),
				os.Getenv("EXOSCALE_API_SECRET"),
				egoscale.ClientOptCond(func() bool {
					if v := os.Getenv("EXOSCALE_TRACE"); v != "" {
						return true
					}
					return false
				}, egoscale.ClientOptWithTrace()))
			if err != nil {
				t.Fatalf("unable to initialize Exoscale client: %s", err)
			}

			defaultSecurityGroup, err = client.FindSecurityGroup(
				exoapi.WithEndpoint(
					context.Background(),
					exoapi.NewReqEndpoint(os.Getenv("EXOSCALE_API_ENVIRONMENT"), testZoneName)),
				testZoneName,
				"default",
			)
			if err != nil {
				t.Fatalf("unable to retrieve default Security Group: %s", err)
			}
		},
		ProviderFactories: testAccProviders,
		Steps: []resource.TestStep{
			{
				Config: testAccResourceSecurityGroupRulesConfigCreate,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckResourceSecurityGroupExists("exoscale_security_group.test", testSecurityGroup),
					testAccCheckResourceSecurityGroupExists("data.exoscale_security_group.default", defaultSecurityGroup),
					func(_ *terraform.State) error {
						require.NotNil(t, testSecurityGroup.ID)
						require.NotNil(t, defaultSecurityGroup.ID)
						return nil
					},
					testAccCheckSecurityGroupHasManyRules(19),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Network:       mustParseCIDR(t, "0.0.0.0/0"),
						Protocol:      nonEmptyStringPtr("icmp"),
						ICMPCode:      &testAccResourceSecurityGroupRulesICMPCode0,
						ICMPType:      &testAccResourceSecurityGroupRulesICMPType8,
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Network:       mustParseCIDR(t, "::/0"),
						Protocol:      nonEmptyStringPtr("icmpv6"),
						ICMPType:      &testAccResourceSecurityGroupRulesICMPv6Type128,
						ICMPCode:      &testAccResourceSecurityGroupRulesICMPCode0,
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Network:       mustParseCIDR(t, "10.0.0.0/24"),
						StartPort:     portValPtr(22),
						EndPort:       portValPtr(22),
						Protocol:      nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Network:       mustParseCIDR(t, "::/0"),
						StartPort:     portValPtr(22),
						EndPort:       portValPtr(22),
						Protocol:      nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Network:       mustParseCIDR(t, "10.0.0.0/24"),
						StartPort:     portValPtr(8000),
						EndPort:       portValPtr(8888),
						Protocol:      nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Network:       mustParseCIDR(t, "::/0"),
						StartPort:     portValPtr(8000),
						EndPort:       portValPtr(8888),
						Protocol:      nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						SecurityGroupID: testSecurityGroup.ID,
						StartPort:       portValPtr(22),
						EndPort:         portValPtr(22),
						Protocol:        nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(22),
						EndPort:         portValPtr(22),
						Protocol:        nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						SecurityGroupID: testSecurityGroup.ID,
						StartPort:       portValPtr(8000),
						EndPort:         portValPtr(8888),
						Protocol:        nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(8000),
						EndPort:         portValPtr(8888),
						Protocol:        nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Protocol:      nonEmptyStringPtr("esp"),
						Network:       mustParseCIDR(t, "192.168.0.0/24"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Protocol:      nonEmptyStringPtr("esp"),
						Network:       mustParseCIDR(t, "::/0"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						Protocol:        nonEmptyStringPtr("esp"),
						SecurityGroupID: defaultSecurityGroup.ID,
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("egress"),
						Network:       mustParseCIDR(t, "192.168.0.0/24"),
						StartPort:     portValPtr(44),
						EndPort:       portValPtr(44),
						Protocol:      nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("egress"),
						Network:       mustParseCIDR(t, "::/0"),
						StartPort:     portValPtr(44),
						EndPort:       portValPtr(44),
						Protocol:      nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("egress"),
						Network:       mustParseCIDR(t, "192.168.0.0/24"),
						StartPort:     portValPtr(2375),
						EndPort:       portValPtr(2377),
						Protocol:      nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("egress"),
						Network:       mustParseCIDR(t, "::/0"),
						StartPort:     portValPtr(2375),
						EndPort:       portValPtr(2377),
						Protocol:      nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("egress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(44),
						EndPort:         portValPtr(44),
						Protocol:        nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("egress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(2375),
						EndPort:         portValPtr(2377),
						Protocol:        nonEmptyStringPtr("udp"),
					}),
				),
			},
			{
				Config: testAccResourceSecurityGroupRulesConfigUpdate,
				Check: resource.ComposeTestCheckFunc(
					testAccCheckResourceSecurityGroupExists("exoscale_security_group.test", testSecurityGroup),
					testAccCheckResourceSecurityGroupExists("data.exoscale_security_group.default", defaultSecurityGroup),
					func(_ *terraform.State) error {
						require.NotNil(t, testSecurityGroup.ID)
						require.NotNil(t, defaultSecurityGroup.ID)
						return nil
					},
					testAccCheckResourceSecurityGroupExists("exoscale_security_group.test", testSecurityGroup),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						SecurityGroupID: testSecurityGroup.ID,
						StartPort:       portValPtr(2222),
						EndPort:         portValPtr(2222),
						Protocol:        nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(2222),
						EndPort:         portValPtr(2222),
						Protocol:        nonEmptyStringPtr("tcp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("egress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(44),
						EndPort:         portValPtr(44),
						Protocol:        nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Protocol:      nonEmptyStringPtr("esp"),
						Network:       mustParseCIDR(t, "192.168.0.0/24"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("ingress"),
						Protocol:      nonEmptyStringPtr("esp"),
						Network:       mustParseCIDR(t, "::/0"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("ingress"),
						Protocol:        nonEmptyStringPtr("esp"),
						SecurityGroupID: testSecurityGroup.ID,
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection: nonEmptyStringPtr("egress"),
						Network:       mustParseCIDR(t, "::/0"),
						StartPort:     portValPtr(44),
						EndPort:       portValPtr(44),
						Protocol:      nonEmptyStringPtr("udp"),
					}),
					testAccCheckSecurityGroupRuleExists(testSecurityGroup, &egoscale.SecurityGroupRule{
						FlowDirection:   nonEmptyStringPtr("egress"),
						SecurityGroupID: defaultSecurityGroup.ID,
						StartPort:       portValPtr(44),
						EndPort:         portValPtr(44),
						Protocol:        nonEmptyStringPtr("udp"),
					}),
				),
			},
		},
	})
}

func testAccCheckSecurityGroupHasManyRules(expected int) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, rs := range s.RootModule().Resources {
			if rs.Type != "exoscale_security_group_rules" {
				continue
			}

			actual := 0
			for k, v := range rs.Primary.Attributes {
				if strings.HasSuffix(k, ".ids.#") {
					count, _ := strconv.Atoi(v)
					actual += count
				}
			}

			if actual != expected {
				return fmt.Errorf("number of rules doesn't match, want %d != got %d", expected, actual)
			}

			return nil
		}

		return errors.New("could not find any Security Group rules")
	}
}

func testAccCheckSecurityGroupRuleExists(
	securityGroup *egoscale.SecurityGroup,
	expected *egoscale.SecurityGroupRule,
) resource.TestCheckFunc {
	return func(s *terraform.State) error {
		for _, r := range securityGroup.Rules {
			if *r.FlowDirection == *expected.FlowDirection && *r.Protocol == *expected.Protocol {
				if strings.HasPrefix(*r.Protocol, "icmp") &&
					*r.ICMPCode == *expected.ICMPCode && *r.ICMPType == *expected.ICMPType {
					return nil
				}

				if r.StartPort != nil && expected.StartPort != nil &&
					*r.StartPort == *expected.StartPort && *r.EndPort == *expected.EndPort {
					return nil
				}

				if r.Network != nil && expected.Network != nil &&
					r.Network.String() == expected.Network.String() {
					return nil
				}

				if defaultString(r.SecurityGroupID, "") == defaultString(expected.SecurityGroupID, "") {
					return nil
				}
			}
		}

		return fmt.Errorf("rule %s not found",
			fmt.Sprintf(
				"%s/protocol=%s/startport=%d/endport=%d/%s",
				*expected.FlowDirection,
				func() string {
					if strings.HasPrefix(*expected.Protocol, "icmp") {
						return fmt.Sprintf(
							"%s(type:%d,code:%d)",
							*expected.Protocol,
							*expected.ICMPType,
							*expected.ICMPType,
						)
					}
					return *expected.Protocol
				}(),
				func() uint16 {
					if expected.StartPort != nil {
						return *expected.StartPort
					}
					return 0
				}(),
				func() uint16 {
					if expected.EndPort != nil {
						return *expected.EndPort
					}
					return 0
				}(),
				func() string {
					if expected.Network != nil {
						return "network=" + expected.Network.String()
					} else {
						return "securitygroupid=" + *expected.SecurityGroupID
					}
				}(),
			),
		)
	}
}

func TestAccCheckSecurityGroupRuleMigrationRuleSerialization(t *testing.T) {
	tests := []struct {
		name     string
		raw      map[string]interface{}
		computed map[string]interface{}
	}{
		{
			name:     "Serialization for schema v0",
			raw:      testAccCheckSecurityGroupRuleMigrationStateDataV0Raw(),
			computed: testAccCheckSecurityGroupRuleMigrationStateDataV0struct(),
		},
		{
			name:     "Serialization for schema v1",
			raw:      testAccCheckSecurityGroupRuleMigrationStateDataV1Raw(),
			computed: testAccCheckSecurityGroupRuleMigrationStateDataV1struct(),
		},
	}

	for _, test := range tests {
		if !reflect.DeepEqual(test.raw, test.computed) {
			t.Fatalf("rule state serialization (%s): \nexpected: '%#v' \ngot:      '%#v'", test.name, test.raw, test.computed)
		}
	}
}

func TestAccCheckSecurityGroupRuleMigrationSucceed(t *testing.T) {
	tests := []struct {
		name        string
		migrated    map[string]interface{}
		legacy      map[string]interface{}
		migrateFunc func(_ context.Context, rawState map[string]interface{}, _ interface{}) (map[string]interface{}, error)
	}{
		{
			name:        "Migrate raw state from v0",
			migrated:    testAccCheckSecurityGroupRuleMigrationStateDataV1(),
			legacy:      testAccCheckSecurityGroupRuleMigrationStateDataV0Raw(),
			migrateFunc: resourceSecurityGroupRulesStateUpgradeV0,
		},
		{
			name:        "Migrate struct-computed state from v0",
			migrated:    testAccCheckSecurityGroupRuleMigrationStateDataV1(),
			legacy:      testAccCheckSecurityGroupRuleMigrationStateDataV0struct(),
			migrateFunc: resourceSecurityGroupRulesStateUpgradeV0,
		},
		{
			name:        "Migrate raw state from v1",
			migrated:    testAccCheckSecurityGroupRuleMigrationStateDataV2(),
			legacy:      testAccCheckSecurityGroupRuleMigrationStateDataV1Raw(),
			migrateFunc: resourceSecurityGroupRulesStateUpgradeV1,
		},
		{
			name:        "Migrate struct-computed state from v1",
			migrated:    testAccCheckSecurityGroupRuleMigrationStateDataV2(),
			legacy:      testAccCheckSecurityGroupRuleMigrationStateDataV1struct(),
			migrateFunc: resourceSecurityGroupRulesStateUpgradeV1,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			migratedState, err := test.migrateFunc(context.Background(), test.legacy, nil)
			if err != nil {
				t.Fatalf("error migrating state: %s", err)
			}

			if !reflect.DeepEqual(test.migrated, migratedState) {
				t.Fatalf("migration error (%s): expected: '%#v' \ngot: '%#v'", test.name, test.migrated, migratedState)
			}
		})
	}
}

// For migration from v0

func testAccCheckSecurityGroupRuleMigrationStateDataV0Raw() map[string]interface{} {
	return map[string]interface{}{
		"ingress": []interface{}{
			map[string]interface{}{
				"cidr_list": []interface{}{
					"0.0.0.0/0",
				},
				"description": "",
				"ids": []interface{}{
					"d7ffd6ff-9788-4834-a44d-3dc49149bfc6_tcp_0.0.0.0/0_0-65535",
				},
				"ports": []interface{}{
					"0-65535",
				},
				"protocol": "TCP",
			},
		},
	}
}

func testAccCheckSecurityGroupRuleMigrationStateDataV0struct() map[string]interface{} {
	legacyRule := stateSecurityGroupRule{
		CIDRList:    []string{"0.0.0.0/0"},
		Description: "",
		IDs:         []string{"d7ffd6ff-9788-4834-a44d-3dc49149bfc6_tcp_0.0.0.0/0_0-65535"},
		Ports:       []string{"0-65535"},
		Protocol:    "TCP",
	}
	legacyRuleInterface, _ := legacyRule.toInterface()

	return map[string]interface{}{
		"ingress": []interface{}{
			legacyRuleInterface,
		},
	}
}

func testAccCheckSecurityGroupRuleMigrationStateDataV1() map[string]interface{} {
	return map[string]interface{}{
		"ingress": []interface{}{
			map[string]interface{}{
				"cidr_list": []interface{}{
					"0.0.0.0/0",
				},
				"description": "",
				"ids": []interface{}{
					"d7ffd6ff-9788-4834-a44d-3dc49149bfc6_tcp_0.0.0.0/0_1-65535",
				},
				"ports": []interface{}{
					"1-65535",
				},
				"protocol": "TCP",
			},
		},
	}
}

// For migration from v1

func testAccCheckSecurityGroupRuleMigrationStateDataV1Raw() map[string]interface{} {
	return map[string]interface{}{
		"ingress": []interface{}{
			map[string]interface{}{
				"user_security_group_list": []interface{}{
					"Test",
				},

				"description": "",
				"ids": []interface{}{
					"d7ffd6ff-9788-4834-a44d-3dc49149bfc6_tcp_Test_1-65535",
				},
				"ports": []interface{}{
					"1-65535",
				},
				"protocol": "TCP",
			},
		},
	}
}

func testAccCheckSecurityGroupRuleMigrationStateDataV1struct() map[string]interface{} {
	legacyRule := stateSecurityGroupRule{
		UserSecurityGroupList: []string{"Test"},
		Description:           "",
		IDs:                   []string{"d7ffd6ff-9788-4834-a44d-3dc49149bfc6_tcp_Test_1-65535"},
		Ports:                 []string{"1-65535"},
		Protocol:              "TCP",
	}
	legacyRuleInterface, _ := legacyRule.toInterface()

	return map[string]interface{}{
		"ingress": []interface{}{
			legacyRuleInterface,
		},
	}
}

func testAccCheckSecurityGroupRuleMigrationStateDataV2() map[string]interface{} {
	return map[string]interface{}{
		"ingress": []interface{}{
			map[string]interface{}{
				"user_security_group_list": []interface{}{
					"test",
				},
				"description": "",
				"ids": []interface{}{
					"d7ffd6ff-9788-4834-a44d-3dc49149bfc6_tcp_test_1-65535",
				},
				"ports": []interface{}{
					"1-65535",
				},
				"protocol": "TCP",
			},
		},
	}
}
