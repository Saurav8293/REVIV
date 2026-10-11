package awsops

import (
	"context"
	"encoding/base64"
	"fmt"
	"math/rand"
	"os"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	awsconfig "github.com/aws/aws-sdk-go-v2/config"
	"github.com/aws/aws-sdk-go-v2/service/ec2"
	ec2types "github.com/aws/aws-sdk-go-v2/service/ec2/types"

	"REVIV/internal/config"
)

// ProvisionStandbyCompute provisions an on-demand cloud compute instance to take over primary workload.
func ProvisionStandbyCompute(ctx context.Context, cfg *config.Config, userDataScript string) (*StandbyComputeResult, error) {
	// Check if simulation mode is active or credentials are absent
	hasAWSKey := os.Getenv("AWS_ACCESS_KEY_ID") != "" && os.Getenv("AWS_SECRET_ACCESS_KEY") != ""
	if cfg.AWS.SimulateCloudOps || !hasAWSKey {
		return simulateEC2Provisioning(cfg)
	}

	awsCfg, err := awsconfig.LoadDefaultConfig(ctx, awsconfig.WithRegion(cfg.AWS.Region))
	if err != nil {
		return nil, fmt.Errorf("failed to load AWS config for EC2: %w", err)
	}

	ec2Client := ec2.NewFromConfig(awsCfg)

	encodedUserData := base64.StdEncoding.EncodeToString([]byte(userDataScript))

	runInput := &ec2.RunInstancesInput{
		MaxCount:     aws.Int32(1),
		MinCount:     aws.Int32(1),
		InstanceType: ec2types.InstanceType(cfg.AWS.InstanceType),
		UserData:     aws.String(encodedUserData),
		TagSpecifications: []ec2types.TagSpecification{
			{
				ResourceType: ec2types.ResourceTypeInstance,
				Tags: []ec2types.Tag{
					{
						Key:   aws.String("Name"),
						Value: aws.String("REVIV-Standby-Cold-Compute"),
					},
					{
						Key:   aws.String("ManagedBy"),
						Value: aws.String("REVIV-Disaster-Recovery-Engine"),
					},
					{
						Key:   aws.String("Role"),
						Value: aws.String("DisasterStandby"),
					},
				},
			},
		},
	}

	if cfg.AWS.AMIID != "" {
		runInput.ImageId = aws.String(cfg.AWS.AMIID)
	}
	if cfg.AWS.SubnetID != "" {
		runInput.SubnetId = aws.String(cfg.AWS.SubnetID)
	}
	if cfg.AWS.KeyPairName != "" {
		runInput.KeyName = aws.String(cfg.AWS.KeyPairName)
	}
	if cfg.AWS.SecurityGroupID != "" {
		runInput.SecurityGroupIds = []string{cfg.AWS.SecurityGroupID}
	}

	runOutput, err := ec2Client.RunInstances(ctx, runInput)
	if err != nil {
		return nil, fmt.Errorf("ec2 RunInstances call failed: %w", err)
	}

	if len(runOutput.Instances) == 0 {
		return nil, fmt.Errorf("ec2 RunInstances returned no instances")
	}

	instance := runOutput.Instances[0]
	instanceID := aws.ToString(instance.InstanceId)

	// Wait for instance to become running
	waiter := ec2.NewInstanceRunningWaiter(ec2Client)
	describeInput := &ec2.DescribeInstancesInput{
		InstanceIds: []string{instanceID},
	}

	_ = waiter.Wait(ctx, describeInput, 5*time.Minute)

	// Fetch refreshed instance metadata
	descOutput, err := ec2Client.DescribeInstances(ctx, describeInput)
	var pubIP, privIP string
	if err == nil && len(descOutput.Reservations) > 0 && len(descOutput.Reservations[0].Instances) > 0 {
		inst := descOutput.Reservations[0].Instances[0]
		pubIP = aws.ToString(inst.PublicIpAddress)
		privIP = aws.ToString(inst.PrivateIpAddress)
	}

	return &StandbyComputeResult{
		InstanceID:   instanceID,
		PublicIP:     pubIP,
		PrivateIP:    privIP,
		InstanceType: cfg.AWS.InstanceType,
		State:        "running",
		LaunchedAt:   time.Now(),
		IsSimulated:  false,
	}, nil
}

func simulateEC2Provisioning(cfg *config.Config) (*StandbyComputeResult, error) {
	// Realistic simulation pause representing fast cloud hypervisor spin-up
	time.Sleep(1200 * time.Millisecond)

	r := rand.New(rand.NewSource(time.Now().UnixNano()))
	instHex := fmt.Sprintf("%08x%08x", r.Uint32(), r.Uint32())
	pubIP := fmt.Sprintf("13.235.%d.%d", r.Intn(200)+10, r.Intn(250)+1)
	privIP := fmt.Sprintf("10.0.1.%d", r.Intn(200)+10)

	return &StandbyComputeResult{
		InstanceID:   fmt.Sprintf("i-%s", instHex),
		PublicIP:     pubIP,
		PrivateIP:    privIP,
		InstanceType: cfg.AWS.InstanceType,
		State:        "running",
		LaunchedAt:   time.Now(),
		IsSimulated:  true,
	}, nil
}
