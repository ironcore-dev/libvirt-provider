package device

type Plugin interface {
	Claim() (string, error)
	Release(pci string) error
	Init() error
}
