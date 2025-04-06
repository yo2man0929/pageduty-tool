package main

import (
	"context"
	"encoding/json"
	"fmt"
	"log"
	"os"
	"strings"
	"time"

	pagerduty "github.com/PagerDuty/go-pagerduty"
	"github.com/joho/godotenv"
	"github.com/spf13/cobra"
)

type AWSCloudWatchAlarm struct {
	AlarmName       string `json:"AlarmName"`
	Region          string `json:"Region"`
	NewStateReason  string `json:"NewStateReason"`
	Trigger struct {
		Dimensions []struct {
			Name  string `json:"name"`
			Value string `json:"value"`
		} `json:"Dimensions"`
		Threshold float64 `json:"Threshold"`
	} `json:"Trigger"`
}

func loadEnv() {
	if err := godotenv.Load(".env"); err != nil {
		log.Println(".env not found, using system environment")
	}
}

func normalizeAWSRegion(region string) string {
	switch strings.ToLower(region) {
	case "eu (ireland)", "eu-west-1", "ireland":
		return "eu-west-1"
	case "us west (oregon)", "us-west-2", "oregon":
		return "us-west-2"
	case "us east (n. virginia)", "us-east-1":
		return "us-east-1"
	// 可以繼續加其他區域的映射
	default:
		return "eu-west-1" // 預設區域
	}
}

func getAWSCommand(client *pagerduty.Client, inc pagerduty.Incident, awsAlarm *AWSCloudWatchAlarm) string {
	if strings.Contains(strings.ToLower(inc.Title), "unhealthyhostcount") && awsAlarm != nil {
		var targetGroupArn, region string
		for _, dim := range awsAlarm.Trigger.Dimensions {
			if strings.EqualFold(dim.Name, "TargetGroup") {
				targetGroupArn = dim.Value
			}
		}

		region = normalizeAWSRegion(awsAlarm.Region)
		if region == "" {
			region = "eu-west-1"
		}

		if targetGroupArn != "" {
			nameParts := strings.Split(targetGroupArn, "/")
			if len(nameParts) >= 2 {
				tgShortName := nameParts[1]

				return fmt.Sprintf(`
💻 AWS CLI Command:
aws elbv2 describe-target-health --target-group-arn $(aws elbv2 describe-target-groups --names %s --query 'TargetGroups[].TargetGroupArn' --region %s --profile kashxa --output text) --query 'TargetHealthDescriptions[?TargetHealth.State==unhealthy].Target.Id' --region %s --profile kashxa --output text`,
					tgShortName, region, region)
			}
		}

		return `
💻 AWS CLI Command:
aws elbv2 describe-target-groups --profile kashxa | grep -A 5 'TargetGroupName'`
	}
	return ""
}



func getAWSDimensions(client *pagerduty.Client, inc pagerduty.Incident) (string, *AWSCloudWatchAlarm) {
	ctx := context.Background()
	alertsResp, err := client.ListIncidentAlertsWithContext(ctx, inc.ID, pagerduty.ListIncidentAlertsOptions{})
	if err != nil {
		log.Printf("failed to list alerts: %v", err)
		return "", nil
	}

	for _, alert := range alertsResp.Alerts {
		if strings.Contains(strings.ToLower(alert.Summary), "unhealthyhostcount") {
			body := alert.Body
			var details map[string]interface{}

			if d, ok := body["details"].(map[string]interface{}); ok {
				details = d
			} else if d, ok := body["custom_details"].(map[string]interface{}); ok {
				details = d
			} else {
				details = body
			}

			data, err := json.Marshal(details)
			if err != nil {
				log.Printf("failed to marshal details: %v", err)
				continue
			}

			var alarm AWSCloudWatchAlarm
			if err := json.Unmarshal(data, &alarm); err != nil {
				log.Printf("failed to unmarshal alarm: %v", err)
				continue
			}

			dims := []string{}
			for _, dim := range alarm.Trigger.Dimensions {
				dims = append(dims, fmt.Sprintf("%s: %s", dim.Name, dim.Value))
			}

			return fmt.Sprintf("AWS Dimensions: %s (Threshold: %.2f)", strings.Join(dims, ", "), alarm.Trigger.Threshold), &alarm
		}
	}

	return "", nil
}




func main() {
	loadEnv()

	var hours int
	var service string
	var status string
	var team string

	rootCmd := &cobra.Command{
		Use:   "pageduty-tool",
		Short: "List PagerDuty incidents",
		Run: func(cmd *cobra.Command, args []string) {
			apiToken := os.Getenv("PAGERDUTY_API_KEY")
			email := os.Getenv("PAGERDUTY_EMAIL")

			if apiToken == "" || email == "" {
				log.Fatal("Set PAGERDUTY_API_KEY and PAGERDUTY_EMAIL in environment or .env")
			}

			client := pagerduty.NewClient(apiToken)

			until := time.Now()
			since := until.Add(-time.Duration(hours) * time.Hour)

			statuses := []string{"triggered", "acknowledged", "resolved"}
			if status != "all" {
				statuses = []string{status}
			}

			opts := pagerduty.ListIncidentsOptions{
				Since:    since.Format(time.RFC3339),
				Until:    until.Format(time.RFC3339),
				Statuses: statuses,
				Limit:    100,
				Offset:   0,
				Total:    true,
			}

			fmt.Printf("\nFetching incidents for the last %d hours...\n", hours)
			fmt.Printf("Service filter: %s\n", service)
			fmt.Printf("Team filter: %s\n", team)
			fmt.Printf("Status filter: %s\n", status)
			fmt.Println("----------------------------------------")

			var allIncidents []pagerduty.Incident
			for {
				incidents, err := client.ListIncidents(opts)
				if err != nil {
					log.Fatalf("Failed to retrieve incidents: %v", err)
				}

				allIncidents = append(allIncidents, incidents.Incidents...)

				if !incidents.More {
					break
				}
				opts.Offset += opts.Limit
			}

			fmt.Printf("Total incidents found: %d\n", len(allIncidents))
			fmt.Println("----------------------------------------")

			for _, inc := range allIncidents {
				if service != "all" && !strings.EqualFold(service, inc.Service.Summary) {
					continue
				}

				if team != "all" && len(inc.Teams) > 0 {
					teamMatch := false
					for _, t := range inc.Teams {
						if strings.EqualFold(team, t.Summary) {
							teamMatch = true
							break
						}
					}
					if !teamMatch {
						continue
					}
				}

				assignees := "None"
				if len(inc.Assignments) > 0 {
					var names []string
					for _, a := range inc.Assignments {
						names = append(names, a.Assignee.Summary)
					}
					assignees = strings.Join(names, ", ")
				}

				statusEmoji := "🔴"
				switch inc.Status {
				case "resolved":
					statusEmoji = "✅"
				case "acknowledged":
					statusEmoji = "⚠️"
				}

				fmt.Printf("─%.0s", strings.Repeat("─", 60))
				fmt.Printf("\n%s Incident: #%d\n", statusEmoji, inc.IncidentNumber)
				fmt.Printf("📌 Status: %s | ⏰ Created: %s\n", inc.Status, inc.CreatedAt)
				if inc.ResolvedAt != "" {
					fmt.Printf("✅ Resolved: %s\n", inc.ResolvedAt)
				}
				fmt.Printf("📚 Title: %s\n", inc.Title)
				fmt.Printf("📌 Service: %s\n", inc.Service.Summary)
				if len(inc.Teams) > 0 {
					var teamNames []string
					for _, t := range inc.Teams {
						teamNames = append(teamNames, t.Summary)
					}
					fmt.Printf("👥 Teams: %s\n", strings.Join(teamNames, ", "))
				}
				fmt.Printf("👤 Assignee(s): %s\n", assignees)
				if inc.Urgency != "" {
					fmt.Printf("⚡ Urgency: %s\n", inc.Urgency)
				}
				if inc.Priority != nil {
					fmt.Printf("🎯 Priority: %s\n", inc.Priority.Summary)
				}

				if awsDims, awsAlarm := getAWSDimensions(client, inc); awsDims != "" {
					fmt.Printf("🌐 %s\n", awsDims)
				
					if awsCmd := getAWSCommand(client, inc, awsAlarm); awsCmd != "" {
						fmt.Printf("%s\n", awsCmd)
					}
				}
			}
		},
	}

	rootCmd.Flags().IntVar(&hours, "hours", 24, "Incident look-back hours")
	rootCmd.Flags().StringVar(&service, "service", "all", "Filter incidents by service (use 'all' for all services)")
	rootCmd.Flags().StringVar(&team, "team", "all", "Filter incidents by team (use 'all' for all teams)")
	rootCmd.Flags().StringVar(&status, "status", "all", "Filter incidents by status (triggered/acknowledged/resolved/all)")
	rootCmd.Execute()
}
