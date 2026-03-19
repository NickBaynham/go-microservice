package lib

import (
	"fmt"

	"github.com/aws/aws-cdk-go/awscdk/v2/awscertificatemanager"
	"github.com/aws/aws-cdk-go/awscdk/v2/awscloudwatch"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsec2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecr"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsecspatterns"
	"github.com/aws/aws-cdk-go/awscdk/v2/awselasticloadbalancingv2"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsiam"
	"github.com/aws/aws-cdk-go/awscdk/v2/awslogs"
	"github.com/aws/aws-cdk-go/awscdk/v2/awsssm"
)

// ── Stack Props ───────────────────────────────────────────────────────────────

type GoMicroserviceStackProps struct {
	awscdk.StackProps
	Environment       string
	AppImage          string // Full ECR URI with tag
	JwtSecret         string // Passed via env var, stored in SSM
	AcmCertificateArn string
	DesiredCount      float64
	TaskCpu           float64
	TaskMemory        float64
}

// ── Stack ─────────────────────────────────────────────────────────────────────

func NewGoMicroserviceStack(scope constructs.Construct, id string, props *GoMicroserviceStackProps) awscdk.Stack {
	stack := awscdk.NewStack(scope, &id, &props.StackProps)

	prefix := fmt.Sprintf("go-microservice-%s", props.Environment)

	// ── ECR Repository ────────────────────────────────────────────────────────
	// Shared across environments — only created once, looked up by name.

	repo := awsecr.NewRepository(stack, jsii.String("ECRRepo"), &awsecr.RepositoryProps{
		RepositoryName:     jsii.String("go-microservice"),
		ImageScanOnPush:    jsii.Bool(true),
		ImageTagMutability: awsecr.TagMutability_MUTABLE,
		LifecycleRules: &[]*awsecr.LifecycleRule{
			{
				MaxImageCount: jsii.Number(10),
				Description:   jsii.String("Keep last 10 images"),
			},
		},
		RemovalPolicy: awscdk.RemovalPolicy_RETAIN, // never delete the registry on destroy
	})

	// ── VPC ───────────────────────────────────────────────────────────────────

	vpc := awsec2.NewVpc(stack, jsii.String("VPC"), &awsec2.VpcProps{
		VpcName:     jsii.String(prefix + "-vpc"),
		MaxAzs:      jsii.Number(2),
		NatGateways: jsii.Number(0), // no NAT = no cost; Fargate uses public subnets
		SubnetConfiguration: &[]*awsec2.SubnetConfiguration{
			{
				Name:       jsii.String("public"),
				SubnetType: awsec2.SubnetType_PUBLIC,
				CidrMask:   jsii.Number(24),
			},
		},
	})

	// ── ECS Cluster ───────────────────────────────────────────────────────────

	cluster := awsecs.NewCluster(stack, jsii.String("Cluster"), &awsecs.ClusterProps{
		ClusterName:       jsii.String(prefix + "-cluster"),
		Vpc:               vpc,
		ContainerInsights: jsii.Bool(true),
	})

	// ── SSM Parameter for JWT Secret ──────────────────────────────────────────

	jwtParam := awsssm.NewStringParameter(stack, jsii.String("JWTSecret"), &awsssm.StringParameterProps{
		ParameterName: jsii.String(fmt.Sprintf("/%s/jwt-secret", prefix)),
		StringValue:   jsii.String(props.JwtSecret),
		Description:   jsii.String("JWT signing secret for go-microservice"),
		Tier:          awsssm.ParameterTier_STANDARD,
	})

	// ── Task Execution Role ───────────────────────────────────────────────────

	executionRole := awsiam.NewRole(stack, jsii.String("ExecutionRole"), &awsiam.RoleProps{
		RoleName:  jsii.String(prefix + "-execution-role"),
		AssumedBy: awsiam.NewServicePrincipal(jsii.String("ecs-tasks.amazonaws.com"), nil),
		ManagedPolicies: &[]awsiam.IManagedPolicy{
			awsiam.ManagedPolicy_FromAwsManagedPolicyName(
				jsii.String("service-role/AmazonECSTaskExecutionRolePolicy"),
			),
		},
	})

	// Allow execution role to read the JWT secret from SSM
	jwtParam.GrantRead(executionRole)

	// ── Task Definition ───────────────────────────────────────────────────────
	// App container + MongoDB sidecar in same task (share localhost network).

	taskDef := awsecs.NewFargateTaskDefinition(stack, jsii.String("TaskDef"), &awsecs.FargateTaskDefinitionProps{
		Family:         jsii.String(prefix + "-task"),
		Cpu:            jsii.Number(props.TaskCpu),
		MemoryLimitMiB: jsii.Number(props.TaskMemory),
		ExecutionRole:  executionRole,
	})

	// App log group
	appLogGroup := awslogs.NewLogGroup(stack, jsii.String("AppLogs"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/ecs/%s/app", prefix)),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	// MongoDB log group
	mongoLogGroup := awslogs.NewLogGroup(stack, jsii.String("MongoLogs"), &awslogs.LogGroupProps{
		LogGroupName:  jsii.String(fmt.Sprintf("/ecs/%s/mongo", prefix)),
		Retention:     awslogs.RetentionDays_ONE_WEEK,
		RemovalPolicy: awscdk.RemovalPolicy_DESTROY,
	})

	// App container
	appContainer := taskDef.AddContainer(jsii.String("app"), &awsecs.ContainerDefinitionOptions{
		Image:     awsecs.ContainerImage_FromRegistry(jsii.String(props.AppImage), nil),
		Essential: jsii.Bool(true),
		Environment: &map[string]*string{
			"PORT":             jsii.String("8080"),
			"TLS_PORT":         jsii.String("8443"),
			"ENV":              jsii.String(props.Environment),
			"MONGO_URI":        jsii.String("mongodb://localhost:27017"),
			"MONGO_DB":         jsii.String("userservice"),
			"JWT_EXPIRE_HOURS": jsii.String("24"),
			"TLS_CERT":         jsii.String("/app/certs/dev-cert.pem"),
			"TLS_KEY":          jsii.String("/app/certs/dev-key.pem"),
		},
		Secrets: &map[string]awsecs.Secret{
			"JWT_SECRET": awsecs.Secret_FromSsmParameter(jwtParam),
		},
		Logging: awsecs.LogDriver_AwsLogs(&awsecs.AwsLogDriverProps{
			LogGroup:     appLogGroup,
			StreamPrefix: jsii.String("app"),
		}),
		HealthCheck: &awsecs.HealthCheck{
			Command: &[]*string{
				jsii.String("CMD-SHELL"),
				jsii.String("wget -qO- --no-check-certificate https://localhost:8443/health || exit 1"),
			},
			Interval:    awscdk.Duration_Seconds(jsii.Number(30)),
			Timeout:     awscdk.Duration_Seconds(jsii.Number(10)),
			Retries:     jsii.Number(3),
			StartPeriod: awscdk.Duration_Seconds(jsii.Number(60)),
		},
	})

	appContainer.AddPortMappings(&awsecs.PortMapping{
		ContainerPort: jsii.Number(8443),
		Protocol:      awsecs.Protocol_TCP,
	})

	// MongoDB sidecar container
	mongoContainer := taskDef.AddContainer(jsii.String("mongo"), &awsecs.ContainerDefinitionOptions{
		Image:     awsecs.ContainerImage_FromRegistry(jsii.String("mongo:7"), nil),
		Essential: jsii.Bool(true),
		Logging: awsecs.LogDriver_AwsLogs(&awsecs.AwsLogDriverProps{
			LogGroup:     mongoLogGroup,
			StreamPrefix: jsii.String("mongo"),
		}),
		HealthCheck: &awsecs.HealthCheck{
			Command: &[]*string{
				jsii.String("CMD-SHELL"),
				jsii.String("mongosh --eval \"db.adminCommand('ping')\" --quiet || exit 1"),
			},
			Interval:    awscdk.Duration_Seconds(jsii.Number(10)),
			Timeout:     awscdk.Duration_Seconds(jsii.Number(5)),
			Retries:     jsii.Number(5),
			StartPeriod: awscdk.Duration_Seconds(jsii.Number(30)),
		},
	})

	mongoContainer.AddPortMappings(&awsecs.PortMapping{
		ContainerPort: jsii.Number(27017),
		Protocol:      awsecs.Protocol_TCP,
	})

	// App must wait for Mongo to be healthy
	appContainer.AddContainerDependencies(&awsecs.ContainerDependency{
		Container: mongoContainer,
		Condition: awsecs.ContainerDependencyCondition_HEALTHY,
	})

	// ── ALB Fargate Service ───────────────────────────────────────────────────
	// ApplicationLoadBalancedFargateService wires up ALB + listener + target group.

	cert := awscertificatemanager.Certificate_FromCertificateArn(
		stack,
		jsii.String("Cert"),
		jsii.String(props.AcmCertificateArn),
	)

	albService := awsecspatterns.NewApplicationLoadBalancedFargateService(
		stack,
		jsii.String("Service"),
		&awsecspatterns.ApplicationLoadBalancedFargateServiceProps{
			ServiceName:            jsii.String(prefix + "-service"),
			Cluster:                cluster,
			TaskDefinition:         taskDef,
			DesiredCount:           jsii.Number(props.DesiredCount),
			AssignPublicIp:         jsii.Bool(true),
			Protocol:               awselasticloadbalancingv2.ApplicationProtocol_HTTPS,
			Certificate:            cert,
			SslPolicy:              awselasticloadbalancingv2.SslPolicy_TLS13_12,
			TargetProtocol:         awselasticloadbalancingv2.ApplicationProtocol_HTTPS,
			HealthCheckGracePeriod: awscdk.Duration_Seconds(jsii.Number(120)),
		},
	)

	// Configure health check on target group
	albService.TargetGroup().ConfigureHealthCheck(&awselasticloadbalancingv2.HealthCheck{
		Path:                    jsii.String("/health"),
		Protocol:                awselasticloadbalancingv2.Protocol_HTTPS,
		HealthyHttpCodes:        jsii.String("200"),
		Interval:                awscdk.Duration_Seconds(jsii.Number(30)),
		Timeout:                 awscdk.Duration_Seconds(jsii.Number(10)),
		HealthyThresholdCount:   jsii.Number(2),
		UnhealthyThresholdCount: jsii.Number(3),
	})

	// HTTP → HTTPS redirect
	albService.LoadBalancer().AddListener(jsii.String("HttpRedirect"), &awselasticloadbalancingv2.BaseApplicationListenerProps{
		Port:     jsii.Number(80),
		Protocol: awselasticloadbalancingv2.ApplicationProtocol_HTTP,
		DefaultAction: awselasticloadbalancingv2.ListenerAction_Redirect(
			&awselasticloadbalancingv2.RedirectOptions{
				Port:       jsii.String("443"),
				Protocol:   jsii.String("HTTPS"),
				StatusCode: awselasticloadbalancingv2.RedirectStatusCode_HTTP_301,
			},
		),
	})

	// Allow ECR pull
	repo.GrantPull(executionRole)

	// ── CloudWatch Dashboard ──────────────────────────────────────────────────

	awscloudwatch.NewDashboard(stack, jsii.String("Dashboard"), &awscloudwatch.DashboardProps{
		DashboardName: jsii.String(prefix + "-dashboard"),
		Widgets: &[][]awscloudwatch.IWidget{
			{
				awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
					Title: jsii.String("ECS CPU Utilization"),
					Left: &[]awscloudwatch.IMetric{
						awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
							Namespace:  jsii.String("AWS/ECS"),
							MetricName: jsii.String("CPUUtilization"),
							DimensionsMap: &map[string]*string{
								"ClusterName": cluster.ClusterName(),
								"ServiceName": albService.Service().ServiceName(),
							},
							Statistic: jsii.String("Average"),
							Period:    awscdk.Duration_Minutes(jsii.Number(1)),
						}),
					},
				}),
				awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
					Title: jsii.String("ECS Memory Utilization"),
					Left: &[]awscloudwatch.IMetric{
						awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
							Namespace:  jsii.String("AWS/ECS"),
							MetricName: jsii.String("MemoryUtilization"),
							DimensionsMap: &map[string]*string{
								"ClusterName": cluster.ClusterName(),
								"ServiceName": albService.Service().ServiceName(),
							},
							Statistic: jsii.String("Average"),
							Period:    awscdk.Duration_Minutes(jsii.Number(1)),
						}),
					},
				}),
			},
			{
				awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
					Title: jsii.String("ALB Request Count"),
					Left: &[]awscloudwatch.IMetric{
						awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
							Namespace:  jsii.String("AWS/ApplicationELB"),
							MetricName: jsii.String("RequestCount"),
							DimensionsMap: &map[string]*string{
								"LoadBalancer": albService.LoadBalancer().LoadBalancerFullName(),
							},
							Statistic: jsii.String("Sum"),
							Period:    awscdk.Duration_Minutes(jsii.Number(1)),
						}),
					},
				}),
				awscloudwatch.NewGraphWidget(&awscloudwatch.GraphWidgetProps{
					Title: jsii.String("ALB 5xx Errors"),
					Left: &[]awscloudwatch.IMetric{
						awscloudwatch.NewMetric(&awscloudwatch.MetricProps{
							Namespace:  jsii.String("AWS/ApplicationELB"),
							MetricName: jsii.String("HTTPCode_Target_5XX_Count"),
							DimensionsMap: &map[string]*string{
								"LoadBalancer": albService.LoadBalancer().LoadBalancerFullName(),
							},
							Statistic: jsii.String("Sum"),
							Period:    awscdk.Duration_Minutes(jsii.Number(1)),
						}),
					},
				}),
			},
		},
	})

	// ── Outputs ───────────────────────────────────────────────────────────────

	awscdk.NewCfnOutput(stack, jsii.String("ALBDnsName"), &awscdk.CfnOutputProps{
		ExportName: jsii.String(prefix + "-alb-dns"),
		Value:      albService.LoadBalancer().LoadBalancerDnsName(),
	})

	awscdk.NewCfnOutput(stack, jsii.String("ECRRepoUrl"), &awscdk.CfnOutputProps{
		ExportName: jsii.String(prefix + "-ecr-url"),
		Value:      repo.RepositoryUri(),
	})

	awscdk.NewCfnOutput(stack, jsii.String("ClusterName"), &awscdk.CfnOutputProps{
		ExportName: jsii.String(prefix + "-cluster-name"),
		Value:      cluster.ClusterName(),
	})

	awscdk.NewCfnOutput(stack, jsii.String("ServiceName"), &awscdk.CfnOutputProps{
		ExportName: jsii.String(prefix + "-service-name"),
		Value:      albService.Service().ServiceName(),
	})

	return stack
}
