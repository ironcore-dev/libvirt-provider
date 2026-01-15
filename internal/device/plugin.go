package device

import (
	"k8s.io/apimachinery/pkg/api/resource"
)

type Plugin interface {
	Claim(quantity resource.Quantity) (string, error)
	Release(pci string) error
	Init() error
}
