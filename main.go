/*
Copyright 2016 Jan Krcmar <honza801@gmail.com>

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package main

import (
	"context"
	"flag"
	"fmt"
	"path/filepath"
	"time"
	"io"
	"os"
	"github.com/klauspost/compress/zstd"
	"gopkg.in/yaml.v2"

	//"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type BackupType struct {
	LabelSelector string `yaml:"labelSelector"`
	Container string
	Command string
	Suffix string
}

type KubackupConfig struct {
	BackupTypes []BackupType `yaml:"backupTypes"`
}

func check(e error) {
    if e != nil {
        panic(e)
    }
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

// The session the S3 Uploader will use
func GetS3Session(s3endpoint string) (sess *session.Session) {
	var err error

	if s3endpoint == "" {
		// Get parameters from environment variables and shared config
		sess, err = session.NewSession()
	} else {
		sess, err = session.NewSession(&aws.Config{
		    Region: aws.String("default"),
		    Endpoint: aws.String(s3endpoint),
		    S3ForcePathStyle: aws.Bool(true),
		})
	}

	if err != nil {
		fmt.Println("GetS3Session error:", err)
	}

	return
}

func UploadS3(sess *session.Session, bucket string, objectname string, reader io.Reader) {
	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	_, err := uploader.Upload(&s3manager.UploadInput{
		Bucket: aws.String(bucket),
		Key: aws.String(objectname),
		Body: reader,
	})
	if err != nil {
		// Print the error and exit.
		fmt.Printf("Unable to upload %q to %q, %v", objectname, bucket, err)
	}

	fmt.Printf("Successfully uploaded %q to %q\n", objectname, bucket)
}

func GetObjectName(p corev1.Pod, suffix string) string {
	currentTime := time.Now()
	return fmt.Sprintf("%s/%s/%s%s", p.Namespace, currentTime.Format("2006-01-02"), p.Name, suffix)
}

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

func GetKubackupConfigFromFile(filename string) (kubackupConfig KubackupConfig){
	configFile, err := os.ReadFile(filename)
	check(err)

	err = yaml.Unmarshal([]byte(configFile), &kubackupConfig)
	check(err)

	return
}

func main() {
	configFile := os.Getenv("KUBACKUP_CONFIG")
	if configFile == "" {
		configFile = "/etc/kubackup/config.yaml"
	}
	kubackupConfig := GetKubackupConfigFromFile(configFile)

	clientset, config := GetInClusterKubernetes()
	if clientset == nil {
		clientset, config = GetKubeConfigKubernetes()
	}

	//os.Setenv("AWS_ACCESS_KEY_ID", "tester")
	//os.Setenv("AWS_SECRET_ACCESS_KEY", "testerpassword")
	//os.Setenv("S3_ENDPOINT", "https://my.minio.test:9000")
	//os.Setenv("S3_BUCKET", "kubackup")
	sess := GetS3Session(os.Getenv("S3_ENDPOINT"))

	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = "kubackup"
	}

	for _, backupType := range kubackupConfig.BackupTypes {
		pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: backupType.LabelSelector})
		check(err)

		for _, p := range pods.Items {

			if p.Status.Phase != corev1.PodRunning {
				continue
			}

			reader, writer := io.Pipe()

			go func() {
				compWriter, err := zstd.NewWriter(writer)
				defer writer.Close()
				defer compWriter.Close()

				err = ExecCmd(clientset, config, p, backupType.Container, backupType.Command, nil, compWriter, os.Stderr)
				if err != nil {
					fmt.Println("ERR", err)
				}
			}()

			defer reader.Close()
			UploadS3(sess, bucket, GetObjectName(p, backupType.Suffix+".zst"), reader)
			time.Sleep(time.Second)
		}

	}
}
