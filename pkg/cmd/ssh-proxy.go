package cmd

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
	batchv1 "k8s.io/api/batch/v1"
	core "k8s.io/api/core/v1"
	v1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"

	"k8s.io/cli-runtime/pkg/genericclioptions"
)

// Cleanup ssh-proxy job and pod immediately after completion
const jobTTL int32 = 0

var (
	use = "%[1]s [flags] ssh|scp|sftp [flags] [arguments]"

	proxyExample = `
	# ssh login to remote system
	%[1]s ssh user@hostname

	# scp secure file copy
	%[1]s scp localpath [user@]host:[path]

	# sftp secure file transfer
	%[1]s sftp [user@]host[:path]
`
)

// ProxyOptions provides information required to
// create a proxy pod in the current context
type ProxyOptions struct {
	configFlags *genericclioptions.ConfigFlags

	userSpecifiedCluster  string
	userSpecifiedContext  string
	userSpecifiedAuthInfo string

	restConfig *rest.Config
	namespace  string
	args       []string

	genericclioptions.IOStreams
}

// NewProxyOptions provides an instance of ProxyOptions with default values
func NewProxyOptions(streams genericclioptions.IOStreams) *ProxyOptions {
	return &ProxyOptions{
		configFlags: genericclioptions.NewConfigFlags(true),

		IOStreams: streams,
	}
}

// NewCmdSshProxy provides a cobra command wrapping ProxyOptions
func NewCmdSshProxy(streams genericclioptions.IOStreams) *cobra.Command {
	o := NewProxyOptions(streams)

	cmd := &cobra.Command{
		Use:          fmt.Sprintf(use, baseName()),
		Short:        "Proxy OpenSSH client tools through Kubernetes pod",
		Example:      fmt.Sprintf(proxyExample, baseName()),
		SilenceUsage: false,
		RunE: func(c *cobra.Command, args []string) error {
			if err := o.Complete(c, args); err != nil {
				return err
			}
			if err := o.Validate(); err != nil {
				return err
			}
			if err := o.Run(args); err != nil {
				return err
			}

			return nil
		},
	}

	//Flag parsing will stop after the first non-flag arg.
	cmd.Flags().SetInterspersed(false)

	o.configFlags.AddFlags(cmd.Flags())

	return cmd
}

// Complete sets all information required for updating the current context
func (o *ProxyOptions) Complete(cmd *cobra.Command, args []string) error {
	o.args = args

	var err error
	o.restConfig, err = o.configFlags.ToRESTConfig()
	if err != nil {
		return err
	}

	o.namespace, _, err = o.configFlags.ToRawKubeConfigLoader().Namespace()
	if err != nil {
		return err
	}

	o.userSpecifiedContext, err = cmd.Flags().GetString("context")
	if err != nil {
		return err
	}

	o.userSpecifiedCluster, err = cmd.Flags().GetString("cluster")
	if err != nil {
		return err
	}

	o.userSpecifiedAuthInfo, err = cmd.Flags().GetString("user")
	if err != nil {
		return err
	}

	return nil
}

// Validate ensures that all required arguments and flag values are provided
func (o *ProxyOptions) Validate() error {
	if len(o.args) < 2 {
		return fmt.Errorf("two or more arguments required")
	}

	return nil
}

// Opens connection through pod
func (o *ProxyOptions) Run(args []string) error {
	clientset, err := kubernetes.NewForConfig(o.restConfig)
	if err != nil {
		return err
	}
	pods, err := clientset.CoreV1().Pods(o.namespace).List(context.TODO(), metav1.ListOptions{LabelSelector: "app=kubectl-ssh-proxy"})
	if err != nil {
		return err
	}
	var sshProxyPod string
	if len(pods.Items) > 0 {
		sshProxyPod = pods.Items[0].Name
	} else {
		fmt.Printf("No proxy pod found in %s namespace\n", o.namespace)
		job := getJobObject()
		job, err := clientset.BatchV1().Jobs(o.namespace).Create(context.TODO(), job, metav1.CreateOptions{})
		if err != nil {
			return err
		}
		sshProxyPod = job.Spec.Template.ObjectMeta.Name
		fmt.Printf("pod/%s created\n", sshProxyPod)
	}

	fmt.Printf("Connecting via pod/%s", sshProxyPod)
	err = waitForPodRunning(clientset, o.namespace, sshProxyPod, 60*time.Second)
	fmt.Print("\n")
	if err != nil {
		return err
	}

	command := args[0]
	commandArgs := args[1:]
	proxyCommand := fmt.Sprintf("ProxyCommand=kubectl --namespace %s exec %s -qi -- nc %%h %%p", o.namespace, sshProxyPod)
	commandArgs = append([]string{"-o", proxyCommand}, commandArgs...)

	switch command {
	case "ssh":
		runCommand("ssh", commandArgs)
	case "scp":
		runCommand("scp", commandArgs)
	case "sftp":
		runCommand("sftp", commandArgs)
	default:
		return fmt.Errorf("unknown command: %s", command)
	}

	return nil
}

func runCommand(program string, commandArgs []string) error {
	cmd := exec.Command(program, commandArgs...)
	cmd.Stdin = os.Stdin
	cmd.Stdout = os.Stdout
	cmd.Stderr = os.Stderr
	fmt.Println(strings.Join(cmd.Args, " "))
	err := cmd.Run()
	return err
}

func baseName() string {
	fileName := filepath.Base(os.Args[0])
	if strings.HasPrefix(fileName, "kubectl-") {
		fileName := "kubectl ssh-proxy"
		return fileName
	}
	return fileName
}

func getJobObject() *batchv1.Job {
	ttl := jobTTL
	return &batchv1.Job{
		ObjectMeta: metav1.ObjectMeta{
			Name: "ssh-proxy",
		},
		Spec: batchv1.JobSpec{
			TTLSecondsAfterFinished: &ttl,
			Template: v1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Name: "ssh-proxy",
					Labels: map[string]string{
						"app": "kubectl-ssh-proxy",
					},
				},
				Spec: v1.PodSpec{
					RestartPolicy: v1.RestartPolicyNever,
					Containers: []core.Container{
						{
							Name:            "busybox",
							Image:           "public.ecr.aws/docker/library/busybox:glibc",
							ImagePullPolicy: core.PullIfNotPresent,
							Command: []string{
								"sleep",
								"12h",
							},
						},
					},
				},
			},
		},
	}
}
