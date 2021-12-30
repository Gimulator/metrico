package main

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"os"
	"path"
	"reflect"
	"runtime"
	"strconv"
	"strings"
	"time"

	log "github.com/sirupsen/logrus"
	"gopkg.in/yaml.v2"
	"gorm.io/gorm"

	corev1 "k8s.io/api/core/v1"
	errors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	k8s "k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	metricsv "k8s.io/metrics/pkg/client/clientset/versioned"

	"github.com/Gimulator/hub/pkg/name"
	"github.com/Gimulator/metrico/pkg/aws"
	sqlite "github.com/Gimulator/metrico/pkg/db"

	// "github.com/Gimulator/metrico/pkg/s3"
	types "github.com/Gimulator/metrico/pkg/types"
)

func init() {
	log.SetFormatter(&log.TextFormatter{
		TimestampFormat:  "2006-01-02 15:04:05",
		FullTimestamp:    true,
		PadLevelText:     true,
		QuoteEmptyFields: true,
		ForceQuote:       false,
		CallerPrettyfier: func(f *runtime.Frame) (string, string) {
			return "", fmt.Sprintf(" %s:%d\t", path.Base(f.File), f.Line)
		},
	})
	log.SetOutput(os.Stdout)
	log.SetLevel(log.DebugLevel) // TODO: get log level from env
}

func main() {
	log.Info("Metrico!")
	ctx := context.TODO()

	log.Info("Validating environment variables")
	envKeys := []string{"METRICO_RUN_ID", "METRICO_NAMESPACE", "METRICO_CONFIGMAP", "METRICO_CONFIGMAP_KEY", "METRICO_S3_URL", "METRICO_S3_ACCESS_KEY", "METRICO_S3_SECRET_KEY"}
	missingKeys := []string{}
	for _, key := range envKeys {
		value, exists := os.LookupEnv(key)
		if !exists || value == "" {
			missingKeys = append(missingKeys, key)
		}
	}
	if len(missingKeys) != 0 {
		log.WithField("keys", missingKeys).Error("The following keys are either not present or have an invalid value. Metrico won't operate without these values.")
		os.Exit(1)
	}

	namespace := os.Getenv("METRICO_NAMESPACE")

	log.Info("Initializing clients")
	config, _ := rest.InClusterConfig()

	log.Info("Initializing kubernetes client")
	clientset, err := k8s.NewForConfig(config)
	if err != nil {
		log.WithField("error", err).Error("Failed to initialize kubernetes client.")
		os.Exit(1)
	}

	log.Info("Initializing metrics client")
	metricsClient, err := metricsv.NewForConfig(config)
	if err != nil {
		log.WithField("error", err).Error("Failed to initialize metrics client.")
		os.Exit(1)
	}

	log.Info("Initializing database")
	db, err := sqlite.NewDatabase(":memory:")

	log.Info("Initializing AWS client")
	aws.Init(os.Getenv("METRICO_S3_URL"), os.Getenv("METRICO_S3_ACCESS_KEY"), os.Getenv("METRICO_S3_SECRET_KEY"))

	log.Info("Getting configs")
	cm, err := clientset.CoreV1().ConfigMaps(namespace).Get(ctx, os.Getenv("METRICO_CONFIGMAP"), metav1.GetOptions{})
	if err != nil {
		log.WithFields(log.Fields{
			"error":     err,
			"configmap": os.Getenv("METRICO_CONFIGMAP"),
			"namespace": namespace,
		}).Error("Failed to get configs from configmap.")
		os.Exit(1)
	}

	metricoConfig := &types.Config{}
	if err := yaml.Unmarshal([]byte(cm.Data[os.Getenv("METRICO_CONFIGMAP_KEY")]), metricoConfig); err != nil {
		log.WithFields(log.Fields{
			"error":     err,
			"configmap": os.Getenv("METRICO_CONFIGMAP"),
			"namespace": os.Getenv("METRICO_NAMESPACE"),
		}).Error("Failed to unmarshal configs from configmap.")
		os.Exit(1)
	}

	podNames := []string{}
	for _, definition := range metricoConfig.Pods {
		podNames = append(podNames, definition.Name)
	}

	if err := WaitForPods(ctx, clientset, namespace, metricoConfig); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("An error occured while waiting for pods.")
		os.Exit(1)
	}

	if err := GetMetrics(ctx, clientset, metricsClient, db, namespace, metricoConfig); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("An error occured while getting pod metrics.")
		os.Exit(1)
	}

	// Putting the metrics to S3
	snapshots, err := sqlite.RetrieveAll(db)
	if err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("An error occured while getting all snapshots.")
		os.Exit(1)
	}

	// allBytes, err := yaml.Marshal(*snapshots)
	// allBytes, err := yaml.Marshal(types.ResourceOutput{
	// 	Data: *snapshots,
	// })
	// if err != nil {
	// 	log.WithFields(log.Fields{
	// 		"error": err,
	// 	}).Error("An error occured while marshaling snapshots.")
	// 	os.Exit(1)
	// }
	// log.WithField("allBytes", string(allBytes)).Info("allBytes")

	// Trying out csv
	var csvArray = [][]string{}
	for i, snapshot := range *snapshots {
		v := reflect.ValueOf(snapshot)
		typeOfS := v.Type()

		if i == 0 {
			// adding headers
			headers := []string{}
			for i := 0; i < v.NumField(); i++ {
				headers = append(headers, typeOfS.Field(i).Name)
			}
			csvArray = append(csvArray, headers)
		}
		row := []string{}
		for i := 0; i < v.NumField(); i++ {
			switch v.Field(i).Interface().(type) {
			case string:
				row = append(row, v.Field(i).Interface().(string))
			case int:
				row = append(row, strconv.Itoa(v.Field(i).Interface().(int)))
			case int64:
				row = append(row, strconv.FormatInt(v.Field(i).Interface().(int64), 10))
			case uint:
				row = append(row, strconv.FormatUint(uint64(v.Field(i).Interface().(uint)), 10))
			}
		}
		csvArray = append(csvArray, row)
	}

	myBytes := new(bytes.Buffer)
	w := csv.NewWriter(myBytes)
	if err := w.WriteAll(csvArray); err != nil {
		log.Fatal(err)
	}

	log.WithField("b", myBytes.String()).Info("b")

	// Putting result to s3
	log.Info("Putting result to s3")
	if err := aws.PutObject(ctx, name.S3LogsBucket(), fmt.Sprintf("%s/metrico.csv", os.Getenv("METRICO_RUN_ID")), strings.NewReader(myBytes.String())); err != nil {
		log.WithFields(log.Fields{
			"error": err,
		}).Error("An error occured while putting data to S3.")
		os.Exit(1)
	}

	log.Info("I'm done here. Peace!")
	os.Exit(0)
}

func ShouldExit(ctx context.Context, clientset *k8s.Clientset, namespace string, config *types.Config) (bool, error) {
	var i int = 0
	for _, podConfig := range config.Pods {
		pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podConfig.Name, metav1.GetOptions{})
		if err != nil {
			if errors.IsNotFound(err) {
				i += 1
			} else {
				return true, err
			}
		} else if pod.Status.Phase != corev1.PodRunning {
			i += 1
		}
	}
	if i == len(config.Pods) {
		return true, nil
	} else {
		return false, nil
	}
}

func WaitForPods(ctx context.Context, clientset *k8s.Clientset, namespace string, config *types.Config) error {
	for {
		try := false
		for _, podConfig := range config.Pods {
			pod, err := clientset.CoreV1().Pods(namespace).Get(ctx, podConfig.Name, metav1.GetOptions{})
			try = (err != nil)
			if err != nil {
				if errors.IsNotFound(err) {
					log.WithField("pod_name", podConfig.Name).Warn("Waiting for pod")
				} else {
					return err
				}
				break
			} else {
				log.WithField("pod_name", pod.Name).Info("Found pod")
			}
		}
		if try {
			time.Sleep(time.Second * 1)
		} else {
			break
		}
	}
	log.Info("All pods are running. Starting to gather metrics.")
	return nil
}

func GetMetrics(ctx context.Context, clientset *k8s.Clientset, metricsClient *metricsv.Clientset, db *gorm.DB, namespace string, config *types.Config) error {
	log.Info("Watching resources ...")
	for {
		podMetrics, err := metricsClient.MetricsV1beta1().PodMetricses(namespace).List(ctx, metav1.ListOptions{})
		if err != nil {
			log.Error("Failed to get namespace metrics list")
			return err
		}
		for _, metric := range podMetrics.Items {
			for _, podConfig := range config.Pods {
				if podConfig.Name == metric.Name {
					for _, containerMetric := range metric.Containers {
						var cpuUsage, cpuSuffix []byte
						cpuUsage, cpuSuffix = containerMetric.Usage.Cpu().CanonicalizeBytes(cpuUsage)

						var memoryUsage, memorySuffix []byte
						memoryUsage, memorySuffix = containerMetric.Usage.Memory().CanonicalizeBytes(memoryUsage)

						sqlite.CleanInsert(db, &types.ResourceSnapshot{
							PodName:       metric.Name,
							ContainerName: containerMetric.Name,
							Timestamp:     metric.Timestamp.Unix(),
							UsageCPU:      fmt.Sprintf("%s%s", string(cpuUsage), string(cpuSuffix)),
							UsageMemory:   fmt.Sprintf("%s%s", string(memoryUsage), string(memorySuffix)),
						})
						log.WithFields(log.Fields{
							"PodName":       metric.Name,
							"ContainerName": containerMetric.Name,
							"Timestamp":     metric.Timestamp.Unix(),
							"UsageCPU":      fmt.Sprintf("%s%s", string(cpuUsage), string(cpuSuffix)),
							"UsageMemory":   fmt.Sprintf("%s%s", string(memoryUsage), string(memorySuffix)),
						}).Info("Captured a resource snapshot")
					}
				}
			}
		}
		shouldExit, err := ShouldExit(ctx, clientset, namespace, config)
		if err != nil {
			log.Error("Failed to get pods' state")
			return err
		}
		if shouldExit {
			return nil
		} else {
			time.Sleep(time.Second * 2)
		}
	}
}
