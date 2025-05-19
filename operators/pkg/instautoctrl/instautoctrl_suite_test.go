package instautoctrl_test

import (
	"path/filepath"
	"testing"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/tools/record"
	"k8s.io/klog/v2/textlogger"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/envtest"

	"github.com/netgroup-polito/CrownLabs/operators/pkg/instautoctrl"
	"github.com/netgroup-polito/CrownLabs/operators/pkg/utils/tests"

	clv1alpha2 "github.com/netgroup-polito/CrownLabs/operators/api/v1alpha2"
)

func TestInstautoctrl(t *testing.T) {
	RegisterFailHandler(Fail)
	RunSpecs(t, "Instautoctrl Suite")
}

var (
	instanceInactiveTerminationReconciler instautoctrl.InstanceInactiveTerminationReconciler
	k8sClient                             client.Client
	testEnv                               = envtest.Environment{
		CRDDirectoryPaths: []string{
			filepath.Join("..", "..", "deploy", "crds"),
			filepath.Join("..", "..", "tests", "crds"),
		},
		ErrorIfCRDPathMissing: true,
	}
	whiteListMap = map[string]string{"production": "true"}
)

var _ = BeforeSuite(func() {
	tests.LogsToGinkgoWriter()

	cfg, err := testEnv.Start()
	Expect(err).ToNot(HaveOccurred())
	Expect(cfg).ToNot(BeNil())

	Expect(clv1alpha2.AddToScheme(scheme.Scheme)).NotTo(HaveOccurred())

	ctrl.SetLogger(textlogger.NewLogger(textlogger.NewConfig()))

	k8sClient, err = client.New(cfg, client.Options{Scheme: scheme.Scheme})
	Expect(err).ToNot(HaveOccurred())
	Expect(k8sClient).NotTo(BeNil())

	instanceInactiveTerminationReconciler = instautoctrl.InstanceInactiveTerminationReconciler{
		Client:             k8sClient,
		Scheme:             scheme.Scheme,
		EventsRecorder:     record.NewFakeRecorder(1024),
		NamespaceWhitelist: metav1.LabelSelector{MatchLabels: whiteListMap},
		ReconcileDeferHook: GinkgoRecover,
	}
})

var _ = AfterSuite(func() {
	Expect(testEnv.Stop()).To(Succeed())
})
