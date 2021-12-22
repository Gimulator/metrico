package types

import (
	corev1 "k8s.io/api/core/v1"
)

type ResourceSnapshot struct { // TODO: change datatypes for resource numbers
	// gorm.Model
	ID            uint `gorm:"primaryKey"`
	PodName       string
	ContainerName string
	Timestamp     int64
	UsageCPU      string
	UsageMemory   string
}

type PodDefinition struct {
	Name      string                 `json:"name" yaml:"name"`
	Resources []*corev1.ResourceName `json:"resources,omitempty" yaml:"resources,omitempty"`
}

type Config struct {
	Pods []PodDefinition `json:"pods" yaml:"pods"`
}

type ResourceOutput struct {
	Data []ResourceSnapshot `json:"data" yaml:"data"`
}
