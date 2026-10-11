package awsops

import (
	"context"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/route53"
	route53types "github.com/aws/aws-sdk-go-v2/service/route53/types"

	"REVIV/internal/config"
)

// SwitchRoute53DNS updates the customer's Route 53 DNS record to redirect traffic to the new standby compute IP.
func SwitchRoute53DNS(ctx context.Context, cfg *config.Config, newTargetIP string) (*Route53SwitchResult, error) {
	hasAWSKey := os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
	if cfg.AWS.SimulateCloudOps || !hasAWSKey {
		return simulateRoute53Switch(cfg, newTargetIP)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWS.Region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for Route53: %w", err)
	}

	r53Client := route53.NewFromConfig(awsCfg)

	recordType := route53types.RRTypeA
	if cfg.AWS.RecordType == "CNAME" {
		recordType = route53types.RRTypeCname
	}

	comment := fmt.Sprintf("REVIV Automated Disaster Recovery DNS cutover at %s", time.Now().Format(time.RFC3339))

	changeInput := &route53.ChangeResourceRecordSetsInput{
		HostedZoneId: aws.String(cfg.AWS.Route53ZoneID),
		ChangeBatch: &route53types.ChangeBatch{
			Comment: aws.String(comment),
			Changes: []route53types.Change{
				{
					Action: route53types.ChangeActionUpsert,
					ResourceRecordSet: &route53types.ResourceRecordSet{
						Name: aws.String(cfg.AWS.Domain),
						Type: recordType,
						TTL:  aws.Int64(60), // Rapid failover TTL
						ResourceRecords: []route53types.ResourceRecord{
							{
								Value: aws.String(newTargetIP),
							},
						},
					},
				},
			},
		},
	}

	resp, err := r53Client.ChangeResourceRecordSets(ctx, changeInput)
	if err != nil {
		return nil, fmt.Errorf("route53 ChangeResourceRecordSets failed: %w", err)
	}

	changeID := ""
	status := "INSYNC"
	if resp.ChangeInfo != nil {
		changeID = aws.ToString(resp.ChangeInfo.Id)
		status = string(resp.ChangeInfo.Status)
	}

	// Optional wait for sync
	waiter := route53.NewResourceRecordSetsChangedWaiter(r53Client)
	_ = waiter.Wait(ctx, &route53.GetChangeInput{Id: aws.String(changeID)}, 45*time.Second)

	return &Route53SwitchResult{
		ZoneID:      cfg.AWS.Route53ZoneID,
		Domain:      cfg.AWS.Domain,
		RecordType:  string(recordType),
		NewTarget:   newTargetIP,
		ChangeID:    changeID,
		Status:      status,
		SwitchedAt:  time.Now(),
		IsSimulated: false,
	}, nil
}

func simulateRoute53Switch(cfg *config.Config, newTargetIP string) (*Route53SwitchResult, error) {
	time.Sleep(800 * time.Millisecond)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	changeID := fmt.Sprintf("/change/C%010X", r.Uint64())

	return &Route53SwitchResult{
		ZoneID:      cfg.AWS.Route53ZoneID,
		Domain:      cfg.AWS.Domain,
		RecordType:  cfg.AWS.RecordType,
		NewTarget:   newTargetIP,
		ChangeID:    changeID,
		Status:      "INSYNC",
		SwitchedAt:  time.Now(),
		IsSimulated: true,
	}, nil
}
