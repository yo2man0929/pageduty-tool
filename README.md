# PagerDuty CLI Tool

A command-line tool to check your assigned PagerDuty incidents with customizable time range.

## Installation


1. Make sure you have Go 1.13 or later installed
2. Clone this repository
3. Copy `.env.example` to `.env` and add your PagerDuty API key and email
4. Run `go mod tidy` to download dependencies
5. Build the tool: `go build`

## Usage

Note: Plase create you own user api key first.

You can provide the PagerDuty API key and your email in two ways:

1. Using the `.env` file:

   ```bash
   # Create .env file with your API key and email
   cp .env.example .env
   # Edit .env and add your API key and email
   vim .env
   ```

2. Using the command-line flags:
   ```bash
   ./pageduty-tool --api-key "your-api-key" --email "your-email@example.com"
   ```

### Time Range Adjustment

By default, the tool shows incidents from the last 24 hours. You can adjust this using the `--hours` flag:

```bash
# Show incidents from the last 12 hours
./pageduty-tool --hours 12

# Show incidents from the last week
./pageduty-tool --hours 168
```

### Filtering Options

You can filter incidents by service, team, or status:

```bash
# Filter by service
./pageduty-tool --service "Infrastructure"

# Filter by team
./pageduty-tool --team "CloudOps"

# Filter by status
./pageduty-tool --status "acknowledged"
```

## AWS Target Group Support

The tool now provides special support for AWS Target Group unhealthy host alerts:

1. For incidents related to `UnhealthyHostCount`, the tool will:
   - Display AWS CloudWatch alarm dimensions (TargetGroup, LoadBalancer, Threshold)
   - Generate a ready-to-use AWS CLI command to check unhealthy instances

Example output for a Target Group alert:

```
Incident: #49193
📌 Status: acknowledged | ⏰ Created: 2025-03-31T23:07:48Z
📚 Title: Average UnHealthyHostCount GreaterThanOrEqualToThreshold 1.0
📌 Service: Infrastructure
👥 Teams: CloudOps
👤 Assignee(s): Cloud Engineer (J)
⚡ Urgency: high
🌐 AWS Dimensions: Threshold: 1.0, TargetGroup: targetgroup/prod-20250331203300989000000002/f5292d70f0366c04, LoadBalancer: app/prod-wptg-graylog-server/7dea8c89f8142b9d

💻 AWS CLI Command:
aws elbv2 describe-target-health --target-group-arn $(aws elbv2 describe-target-groups --names prod-20250331203300989000000002 --query 'TargetGroups[].TargetGroupArn' --region eu-west-1 --profile kashxa --output text) --query 'TargetHealthDescriptions[?TargetHealth.State==`unhealthy`].Target.Id' --region eu-west-1 --profile kashxa --output text
```

## Output

The tool will display all incidents in the specified time range, including:

- Incident ID and number
- Title
- Status with emoji indicators (🔴 triggered, ⚠️ acknowledged, ✅ resolved)
- Priority and Urgency
- Service name
- Team information
- Assignee details
- Creation and resolution times
- AWS specific details for supported alerts
