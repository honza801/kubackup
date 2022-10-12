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
	"compress/gzip"

	//"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	"k8s.io/client-go/util/homedir"

	"k8s.io/client-go/kubernetes/scheme"
	restclient "k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"

	"github.com/aws/aws-sdk-go/aws"
	"github.com/aws/aws-sdk-go/aws/session"
	"github.com/aws/aws-sdk-go/service/s3/s3manager"
)

type DBType struct {
	labelSelector string
	command string
}

func DBTypes() []DBType {
	return []DBType {
		{
			labelSelector: "app.kubernetes.io/name=mariadb",
			command: "mysqldump -u root -p$MARIADB_ROOT_PASSWORD --all-databases",
		},
		{
			labelSelector: "app.kubernetes.io/name=mysql",
			command: "mysqldump -u root -p$MYSQL_ROOT_PASSWORD --all-databases",
		},
	}
}

// ExecCmd exec command on specific pod and wait the command's output.
func ExecCmd(client kubernetes.Interface, config *restclient.Config, pod corev1.Pod,
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
        Stdin:   true,
        Stdout:  true,
        Stderr:  true,
        TTY:     true,
    }
    if stdin == nil {
        option.Stdin = false
    }
    req.VersionedParams(
        option,
        scheme.ParameterCodec,
    )

    fmt.Println("EXEC start on ns:", pod.Namespace, "p:", pod.Name)//, "command:", command)
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

func UploadS3(s3endpoint string, bucket string, objectname string, reader io.Reader) {
	// The session the S3 Uploader will use
	sess, err := session.NewSession(&aws.Config{
	    Region: aws.String("default"),
	    Endpoint: aws.String(s3endpoint),
	    S3ForcePathStyle: aws.Bool(true),
	})

	// Create an uploader with the session and default options
	uploader := s3manager.NewUploader(sess)

	_, err = uploader.Upload(&s3manager.UploadInput{
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

func GetFileName(p corev1.Pod) string {
	return fmt.Sprintf("./tmp/%s-%s.gz", p.Namespace, p.Name)
}

func GetObjectName(p corev1.Pod) string {
	currentTime := time.Now()
	return fmt.Sprintf("%s/%s/%s.gz", p.Namespace, currentTime.Format("2006-01-02"), p.Name)
}

func GetGzipWriter(p corev1.Pod) *gzip.Writer {
	outputFile, err := os.OpenFile(GetFileName(p), os.O_WRONLY|os.O_CREATE, 0644)
	if err != nil {
		fmt.Printf("Err: %s", err)
	}
	gzipFile := gzip.NewWriter(outputFile)
	defer outputFile.Close()
	defer gzipFile.Close()
	return gzipFile
}

func GetKubernetes() (kubernetes.Interface, *restclient.Config) {
	var kubeconfig *string
	if home := homedir.HomeDir(); home != "" {
		kubeconfig = flag.String("kubeconfig", filepath.Join(home, ".kube", "config"), "(optional) absolute path to the kubeconfig file")
	} else {
		kubeconfig = flag.String("kubeconfig", "", "absolute path to the kubeconfig file")
	}
	flag.Parse()

	// use the current context in kubeconfig
	config, err := clientcmd.BuildConfigFromFlags("", *kubeconfig)
	if err != nil {
		panic(err.Error())
	}

	// create the clientset
	clientset, err := kubernetes.NewForConfig(config)
	if err != nil {
		panic(err.Error())
	}

	return clientset, config
}

func main() {
	clientset, config := GetKubernetes()

	//os.Setenv("AWS_ACCESS_KEY_ID", "tester")
	//os.Setenv("AWS_SECRET_ACCESS_KEY", "testerpassword")
	//os.Setenv("S3_ENDPOINT", "https://my.minio.test:9000")
	//os.Setenv("S3_BUCKET", "kubackup")
	s3endpoint := os.Getenv("S3_ENDPOINT")
	bucket := os.Getenv("S3_BUCKET")
	if bucket == "" {
		bucket = "kubackup"
	}

	for _, dbType := range DBTypes() {
		pods, err := clientset.CoreV1().Pods("").List(context.TODO(), metav1.ListOptions{LabelSelector: dbType.labelSelector})
		if err != nil {
			panic(err.Error())
		}

		for _, p := range pods.Items {
			//gzipFile = GetGzipWriter(p)

			reader, writer := io.Pipe()

			go func() {
				gzipFile := gzip.NewWriter(writer)
				defer gzipFile.Close()
				defer writer.Close()

				err = ExecCmd(clientset, config, p, dbType.command, nil, gzipFile, os.Stderr)
				if err != nil {
					fmt.Println("ERR", err)
				}
			}()

			UploadS3(s3endpoint, bucket, GetObjectName(p), reader)
			time.Sleep(time.Second)
			reader.Close()
		}

	}
}
