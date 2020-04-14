// Copyright DataStax, Inc.
// Please see the included license file for details.

package ginkgo_util

import (
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"
	"strconv"
	"sort"

	ginkgo "github.com/onsi/ginkgo"
	. "github.com/onsi/gomega"

	"github.com/datastax/cass-operator/mage/kubectl"
	mageutil "github.com/datastax/cass-operator/mage/util"
)

const (
	EnvNoCleanup = "M_NO_CLEANUP"
)

func duplicate(value string, count int) string {
	result := []string{}
	for i := 0; i < count; i++ {
		result = append(result, value)
	}

	return strings.Join(result, " ")
}


// Wrapper type to make it simpler to
// set a namespace one time and execute all of your
// KCmd objects inside of it, and then use Gomega
// assertions on panic
type NsWrapper struct {
	Namespace     string
	TestSuiteName string
	LogDir        string
	stepCounter   int
}

func NewWrapper(suiteName string, namespace string) NsWrapper {
	return NsWrapper{
		Namespace:     namespace,
		TestSuiteName: suiteName,
		LogDir:        genSuiteLogDir(suiteName),
		stepCounter:   1,
	}

}

func (k NsWrapper) ExecV(kcmd kubectl.KCmd) error {
	err := kcmd.InNamespace(k.Namespace).ExecV()
	return err
}

func (k NsWrapper) ExecVPanic(kcmd kubectl.KCmd) {
	err := kcmd.InNamespace(k.Namespace).ExecV()
	Expect(err).ToNot(HaveOccurred())
}

func (k NsWrapper) Output(kcmd kubectl.KCmd) (string, error) {
	out, err := kcmd.InNamespace(k.Namespace).Output()
	return out, err
}

func (k NsWrapper) OutputPanic(kcmd kubectl.KCmd) string {
	out, err := kcmd.InNamespace(k.Namespace).Output()
	Expect(err).ToNot(HaveOccurred())
	return out
}

func (k NsWrapper) WaitForOutput(kcmd kubectl.KCmd, expected string, seconds int) error {
	return kubectl.WaitForOutput(kcmd.InNamespace(k.Namespace), expected, seconds)
}

func (k NsWrapper) WaitForOutputContains(kcmd kubectl.KCmd, expected string, seconds int) error {
	return kubectl.WaitForOutputContains(kcmd.InNamespace(k.Namespace), expected, seconds)
}

func (k NsWrapper) WaitForOutputPanic(kcmd kubectl.KCmd, expected string, seconds int) {
	err := kubectl.WaitForOutput(kcmd.InNamespace(k.Namespace), expected, seconds)
	Expect(err).ToNot(HaveOccurred())
}

func (k NsWrapper) WaitForOutputContainsPanic(kcmd kubectl.KCmd, expected string, seconds int) {
	err := kubectl.WaitForOutput(kcmd.InNamespace(k.Namespace), expected, seconds)
	Expect(err).ToNot(HaveOccurred())
}

func (k NsWrapper) WaitForOutputPattern(kcmd kubectl.KCmd, pattern string, seconds int) error {
	return kubectl.WaitForOutputPattern(kcmd.InNamespace(k.Namespace), pattern, seconds)
}

func (k *NsWrapper) countStep() int {
	n := k.stepCounter
	k.stepCounter++
	return n
}

func (k NsWrapper) Terminate() error {
	noCleanup := os.Getenv(EnvNoCleanup)
	if strings.ToLower(noCleanup) == "true" {
		fmt.Println("Skipping namespace cleanup and deletion.")
		return nil
	}

	fmt.Println("Cleaning up and deleting namespace.")
	// Always try to delete the dc that was used in the test
	// incase the test failed out before a delete step.
	//
	// This is important because deleting the namespace itself
	// can hang if this step is skipped.
	kcmd := kubectl.Delete("cassandradatacenter", "--all")
	_ = k.ExecV(kcmd)
	return kubectl.DeleteByTypeAndName("namespace", k.Namespace).ExecV()
}

//===================================
// Logging functions for the NsWrapper
// that execute the Kcmd and then dump
// k8s logs for that namespace
//====================================
func sanitizeForLogDirs(s string) string {
	reg, err := regexp.Compile(`[\s\\\/\-\.,]`)
	mageutil.PanicOnError(err)
	return reg.ReplaceAllLiteralString(s, "_")
}

func genSuiteLogDir(suiteName string) string {
	datetime := time.Now().Format("2006.01.02_15:04:05")
	return fmt.Sprintf("../../build/kubectl_dump/%s/%s",
		sanitizeForLogDirs(suiteName), datetime)
}

func (ns *NsWrapper) genTestLogDir(description string) string {
	sanitizedDesc := sanitizeForLogDirs(description)
	return fmt.Sprintf("%s/%02d_%s", ns.LogDir, ns.countStep(), sanitizedDesc)
}

func (ns *NsWrapper) ExecAndLog(description string, kcmd kubectl.KCmd) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.ExecV(kcmd)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) OutputAndLog(description string, kcmd kubectl.KCmd) string {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	output, execErr := ns.Output(kcmd)
	Expect(execErr).ToNot(HaveOccurred())
	return output
}

func (ns *NsWrapper) WaitForOutputAndLog(description string, kcmd kubectl.KCmd, expected string, seconds int) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.WaitForOutput(kcmd, expected, seconds)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) WaitForOutputPatternAndLog(description string, kcmd kubectl.KCmd, expected string, seconds int) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.WaitForOutputPattern(kcmd, expected, seconds)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) WaitForOutputContainsAndLog(description string, kcmd kubectl.KCmd, expected string, seconds int) {
	ginkgo.By(description)
	defer kubectl.DumpLogs(ns.genTestLogDir(description), ns.Namespace).ExecVPanic()
	execErr := ns.WaitForOutputContains(kcmd, expected, seconds)
	Expect(execErr).ToNot(HaveOccurred())
}

func (ns *NsWrapper) WaitForDatacenterToHaveNoPods(dcName string) {
	step := "checking that no dc pods remain"
	json := "jsonpath={.items}"
	k := kubectl.Get("pods").
		WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, "[]", 300)
}

func (ns *NsWrapper) WaitForDatacenterOperatorProgress(dcName string, progressValue string, timeout int) {
	step := fmt.Sprintf("checking the cassandra operator progress status is set to %s", progressValue)
	json := "jsonpath={.status.cassandraOperatorProgress}"
	k := kubectl.Get("CassandraDatacenter", dcName).
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, progressValue, timeout)
}

func (ns *NsWrapper) WaitForDatacenterReadyPodCount(dcName string, count int) {
	timeout := count * 400
	step := "waiting for the node to become ready"
	json := "jsonpath={.items[*].status.containerStatuses[0].ready}"
	k := kubectl.Get("pods").
		WithLabel(fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		WithFlag("field-selector", "status.phase=Running").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, duplicate("true", count), timeout)
}

func (ns *NsWrapper) WaitForDatacenterReady(dcName string) {
	json := "jsonpath={.spec.size}"
	k := kubectl.Get("CassandraDatacenter", dcName).FormatOutput(json)
	sizeString := ns.OutputPanic(k)
	size, err := strconv.Atoi(sizeString)
	Expect(err).ToNot(HaveOccurred())

	ns.WaitForDatacenterReadyPodCount(dcName, size)
	ns.WaitForDatacenterOperatorProgress(dcName, "Ready", 30)
}

func (ns *NsWrapper) WaitForPodNotStarted(podName string) {
	step := "verify that the pod is no longer marked as started"
	k := kubectl.Get("pod").
		WithFlag("field-selector", "metadata.name="+podName).
		WithFlag("selector", "cassandra.datastax.com/node-state=Started")
	ns.WaitForOutputAndLog(step, k, "", 60)
}

func (ns *NsWrapper) WaitForPodStarted(podName string) {
	step := "verify that the pod is marked as started"
	json := "jsonpath={.items[*].metadata.name}"
	k := kubectl.Get("pod").
		WithFlag("field-selector", "metadata.name="+podName).
		WithFlag("selector", "cassandra.datastax.com/node-state=Started").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, podName, 60)
}

func (ns *NsWrapper) DisableGossipWaitNotReady(podName string) {
	ns.DisableGossip(podName)
	ns.WaitForPodNotStarted(podName)
}

func (ns *NsWrapper) EnableGossipWaitReady(podName string) {
	ns.EnableGossip(podName)
	ns.WaitForPodStarted(podName)
}

func (ns *NsWrapper) DisableGossip(podName string) {
	execArgs := []string{"-c", "cassandra",
		"--", "bash", "-c",
		"nodetool disablegossip",
	}
	k := kubectl.ExecOnPod(podName, execArgs...)
	ns.ExecVPanic(k)
}

func (ns *NsWrapper) EnableGossip(podName string) {
	execArgs := []string{"-c", "cassandra",
		"--", "bash", "-c",
		"nodetool enablegossip",
	}
	k := kubectl.ExecOnPod(podName, execArgs...)
	ns.ExecVPanic(k)
}

func (ns *NsWrapper) GetDatacenterPodNames(dcName string) []string {
	json := "jsonpath={.items[*].metadata.name}"
	k := kubectl.Get("pods").
		WithFlag("selector", fmt.Sprintf("cassandra.datastax.com/datacenter=%s", dcName)).
		FormatOutput(json)

	output := ns.OutputPanic(k)
	podNames := strings.Split(output, " ")
	sort.Sort(sort.StringSlice(podNames))

	return podNames
}

func (ns *NsWrapper) WaitForOperatorReady() {
	step := "waiting for the operator to become ready"
	json := "jsonpath={.items[0].status.containerStatuses[0].ready}"
	k := kubectl.Get("pods").
		WithLabel("name=cass-operator").
		WithFlag("field-selector", "status.phase=Running").
		FormatOutput(json)
	ns.WaitForOutputAndLog(step, k, "true", 120)
}