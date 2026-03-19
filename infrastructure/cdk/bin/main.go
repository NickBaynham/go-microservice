package main

import (
	"fmt"
	"go-microservice-cdk/lib"
	"os"
	"strconv"
)

func main() {
	defer jsii.Close()

	app := awscdk.NewApp(nil)

	environment := mustEnv("CDK_ENV")      // dev | test | prod
	appImage := mustEnv("CDK_APP_IMAGE")   // ECR URI with tag
	jwtSecret := mustEnv("CDK_JWT_SECRET") // sensitive — set in CI secrets
	certArn := mustEnv("CDK_CERT_ARN")     // ACM certificate ARN
	awsAccount := mustEnv("CDK_ACCOUNT")   // AWS account ID
	awsRegion := getEnv("CDK_REGION", "us-east-1")

	desiredCount := getFloat("CDK_DESIRED_COUNT", 1)
	taskCpu := getFloat("CDK_TASK_CPU", 512)
	taskMemory := getFloat("CDK_TASK_MEMORY", 1024)

	lib.NewGoMicroserviceStack(app, fmt.Sprintf("GoMicroservice-%s", capitalize(environment)), &lib.GoMicroserviceStackProps{
		StackProps: awscdk.StackProps{
			Env: &awscdk.Environment{
				Account: jsii.String(awsAccount),
				Region:  jsii.String(awsRegion),
			},
			Description: jsii.String(fmt.Sprintf("go-microservice %s environment", environment)),
			Tags: &map[string]*string{
				"Project":     jsii.String("go-microservice"),
				"Environment": jsii.String(environment),
				"ManagedBy":   jsii.String("cdk"),
			},
		},
		Environment:       environment,
		AppImage:          appImage,
		JwtSecret:         jwtSecret,
		AcmCertificateArn: certArn,
		DesiredCount:      desiredCount,
		TaskCpu:           taskCpu,
		TaskMemory:        taskMemory,
	})

	app.Synth(nil)
}

func mustEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		panic(fmt.Sprintf("required environment variable %s is not set", key))
	}
	return v
}

func getEnv(key, fallback string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return fallback
}

func getFloat(key string, fallback float64) float64 {
	v := os.Getenv(key)
	if v == "" {
		return fallback
	}
	f, err := strconv.ParseFloat(v, 64)
	if err != nil {
		return fallback
	}
	return f
}

func capitalize(s string) string {
	if len(s) == 0 {
		return s
	}
	return string(s[0]-32) + s[1:]
}
