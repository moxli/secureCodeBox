// SPDX-FileCopyrightText: the secureCodeBox authors
//
// SPDX-License-Identifier: Apache-2.0

package main_test

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	"github.com/aws/aws-sdk-go/service/sqs"
	"github.com/go-logr/logr"
	"github.com/secureCodeBox/secureCodeBox/auto-discovery/cloud-aws/aws"
	"github.com/secureCodeBox/secureCodeBox/auto-discovery/cloud-aws/config"
	"github.com/secureCodeBox/secureCodeBox/auto-discovery/cloud-aws/kubernetes"
	configv1 "github.com/secureCodeBox/secureCodeBox/auto-discovery/kubernetes/api/v1"
	executionv1 "github.com/secureCodeBox/secureCodeBox/operator/apis/execution/v1"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"
	logf "sigs.k8s.io/controller-runtime/pkg/log"
	"sigs.k8s.io/controller-runtime/pkg/log/zap"
)

// These tests use Ginkgo (BDD-style Go testing framework). Refer to
// http://onsi.github.io/ginkgo/ to learn more about Ginkgo.

// The main tests are integration tests for the whole autodicsovery service, which feed fake
// messages into the running AWSMonitor and then check the results of the actions taken by the
// Reconciler in kubernetes using envtest

const namespace = "go-tests"

var log logr.Logger
var k8sClient client.Client
var testEnv *envtest.Environment
var ctx context.Context
var cancel context.CancelFunc
var sqsapi *MockSQSService
var awsReconciler kubernetes.AWSReconciler
var awsMonitor *aws.MonitorService

func TestIntegration(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Integration Suite")
}

var _ = BeforeSuite(func() {
	log = zap.New(zap.WriteTo(GinkgoWriter), zap.UseDevMode(true))
	logf.SetLogger(log)

	ctx, cancel = context.WithCancel(context.TODO())

	By("bootstrapping test environment")
	testEnv = &envtest.Environment{
		CRDDirectoryPaths:     []string{filepath.Join("..", "..", "operator", "config", "crd", "bases")},
		ErrorIfCRDPathMissing: true,
	}

	cfg, err := testEnv.Start()
	Expect(err).NotTo(HaveOccurred())
	Expect(cfg).NotTo(BeNil())

	Expect(executionv1.AddToScheme(scheme.Scheme)).To(Succeed())

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).NotTo(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	Expect(createNamespace(ctx, namespace)).To(Succeed())

	autoDiscoveryCfg := config.AutoDiscoveryConfig{
		Aws: config.AwsConfig{
			QueueUrl: "notaqueue",
			Region:   "doesnotmatter",
		},
		Kubernetes: config.KubernetesConfig{
			Namespace: namespace,
			ScanConfigs: []configv1.ScanConfig{
				{
					Name:           "test-scan",
					RepeatInterval: metav1.Duration{Duration: time.Hour},
					Annotations:    map[string]string{"testAnnotation": "{{ .Target.Id }}"},
					Labels:         map[string]string{"testLabel": "{{ .Target.Id }}"},
					Parameters:     []string{"{{ .ImageID }}"},
					ScanType:       "trivy-sbom-image",
					HookSelector: metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Operator: metav1.LabelSelectorOpIn,
								Key:      "foo",
								Values:   []string{"bar", "baz"},
							},
							{
								Operator: metav1.LabelSelectorOpDoesNotExist,
								Key:      "foo",
							},
						},
					},
				},
				{
					Name:           "test-scan-two",
					RepeatInterval: metav1.Duration{Duration: time.Hour},
					Annotations:    map[string]string{"testAnnotation": "{{ .Target.Id }}"},
					Labels:         map[string]string{"testLabel": "{{ .Target.Id }}"},
					Parameters:     []string{"{{ .ImageID }}"},
					ScanType:       "trivy-sbom-image",
					HookSelector: metav1.LabelSelector{
						MatchExpressions: []metav1.LabelSelectorRequirement{
							{
								Operator: metav1.LabelSelectorOpIn,
								Key:      "foo",
								Values:   []string{"bar", "baz"},
							},
							{
								Operator: metav1.LabelSelectorOpDoesNotExist,
								Key:      "foo",
							},
						},
					},
				},
			},
		},
	}

	sqsapi = &MockSQSService{
		MsgEntry: make(chan *sqs.ReceiveMessageOutput),
	}

	awsReconciler = kubernetes.NewAWSReconcilerWith(k8sClient, &autoDiscoveryCfg, log)
	awsMonitor = aws.NewMonitorServiceWith(&autoDiscoveryCfg, sqsapi, awsReconciler, log)

	go func() {
		defer GinkgoRecover()
		awsMonitor.Run(ctx)
	}()
})

var _ = AfterSuite(func() {
	cancel()
	By("tearing down the test environment")
	Expect(testEnv.Stop()).To(Succeed())
})

func createNamespace(ctx context.Context, namespaceName string) error {
	namespace := &corev1.Namespace{
		ObjectMeta: metav1.ObjectMeta{
			Name: namespaceName,
		},
	}

	return k8sClient.Create(ctx, namespace)
}

type MockSQSService struct {
	// MsgEntry can be used to insert messages into the mocked sqs interface where they will be
	// retrieved by the AWSMonitor
	MsgEntry chan *sqs.ReceiveMessageOutput
}

func (m *MockSQSService) ReceiveMessage(input *sqs.ReceiveMessageInput) (*sqs.ReceiveMessageOutput, error) {
	timeout := time.After(time.Duration(*input.WaitTimeSeconds) * time.Second)
	for {
		select {
		case <-timeout:
			return &sqs.ReceiveMessageOutput{}, nil
		case msg := <-m.MsgEntry:
			return msg, nil
		}
	}
}

func (*MockSQSService) DeleteMessage(input *sqs.DeleteMessageInput) (*sqs.DeleteMessageOutput, error) {
	// nothing to do because we don't actually store messages during the tests
	return &sqs.DeleteMessageOutput{}, nil
}
