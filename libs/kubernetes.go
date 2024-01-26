package libs

import (
	"flag"
	"fmt"
	"path/filepath"
	"io"

	//"k8s.io/apimachinery/pkg/api/errors"
	//metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

func GetKubeConfigKubernetes() (clientset kubernetes.Interface, config *rest.Config) {
	var kubeconfig *string
	var err error

	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err = clientcmd.BuildConfigFromFlags("", *kubeconfig)
	check(err)

	// create the clientset
	clientset, err = kubernetes.NewForConfig(config)
	check(err)

	return
}

func GetInClusterKubernetes() (clientset kubernetes.Interface, config *rest.Config) {
	var err error

	// creates the in-cluster config
	config, err = rest.InClusterConfig()
	if err != nil {
		return nil, nil
	}

	// creates the clientset
	clientset, err = kubernetes.NewForConfig(config)
	check(err)

	return
}

// ExecCmd exec command on specific pod and wait the command's output.
func ExecCmd(client kubernetes.Interface, config *rest.Config, pod corev1.Pod, container string,
    command string, stdin io.Reader, stdout io.Writer, stderr io.Writer) error {
    cmd := []string{
        "sh",
        "-c",
        command,
    }
    req := client.CoreV1().RESTClient().Post().Resource("pods").Name(pod.Name).
        Namespace(pod.Namespace).SubResource("exec")
    option := &corev1.PodExecOptions{
        Command: cmd,
        Stdout:  true,
        Stderr:  true,
        TTY:     false,
    }
    if stdin == nil {
        option.Stdin = false
    } else {
        option.Stdin = true
    }
    if container != "" {
        option.Container = container
    }
    req.VersionedParams(
        option,
        scheme.ParameterCodec,
    )

    fmt.Println("EXEC start on ns:", pod.Namespace, "p:", pod.Name, "c:", container)//, "cmd:", command)
    exec, err := remotecommand.NewSPDYExecutor(config, "POST", req.URL())
    if err != nil {
	    return err
    }
    err = exec.Stream(remotecommand.StreamOptions{
        Stdin:  stdin,
        Stdout: stdout,
        Stderr: stderr,
    })
    if err != nil {
        return err
    }
    //fmt.Println("EXEC end on ns:", pod.Namespace, "p:", pod.Name)

    return nil
}

