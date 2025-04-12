// Parsers para tratar comandos dos dispositivos
package parsers

import (
	"fmt"
	"strings"
)

// Struct para padronizar a saída dos parsers
type InterfaceFormatted struct {
	Name  string
	Descr string
	IP    string
	Mask  string
	State string
}

// ParseConfig implementa a interface InterfaceParser
// TODO: Implementar a lógica de parsing para também identificar o tipo do OS (XE, XR, NX-OS, etc)
type CiscoIOSParser struct{}

type InterfaceParser interface {
	ParseConfig(interfaceOutput string) (InterfaceFormatted, error)
	ParseStatus(interfaceOutput string) (string, error)
}

// Factory para criar parsers de acordo com o tipo de dispositivo
type InterfaceParserFactory struct {
	DeviceType string
}

// GetParser retorna o parser apropriado com base no tipo de dispositivo
func (f *InterfaceParserFactory) GetParser() (InterfaceParser, error) {
	switch f.DeviceType {
	case "cisco_ios":
		return &CiscoIOSParser{}, nil
	default:
		return nil, fmt.Errorf("parser not found for device type: %s", f.DeviceType)
	}
}

func (p *CiscoIOSParser) ParseConfig(interfaceOutput string) (InterfaceFormatted, error) {
	// Exemplo de parsing simples
	// Aqui você deve implementar a lógica real de parsing para o Cisco IOS
	parsedConfig := InterfaceFormatted{}
	lines := strings.Split(interfaceOutput, "\n")
	for _, line := range lines {
		if strings.Contains(line, "description") {
			parsedConfig.Descr = strings.TrimSpace(strings.Split(line, "description")[1])
		} else if strings.Contains(line, "ip address") {
			parsedConfig.IP = strings.Split(strings.TrimSpace(strings.Split(line, "ip address")[1]), " ")[0]
			parsedConfig.Mask = strings.Split(strings.TrimSpace(strings.Split(line, "ip address")[1]), " ")[1]
		}
		if strings.Contains(line, "interface") {
			parsedConfig.Name = strings.TrimSpace(strings.Split(line, "interface")[1])
		}

	}
	return parsedConfig, nil
}

func (p *CiscoIOSParser) ParseStatus(interfaceOutput string) (string, error) {
	// Exemplo de parsing simples
	// Aqui você deve implementar a lógica real de parsing para o Cisco IOS
	lines := strings.Split(interfaceOutput, "\n")
	for _, line := range lines {
		if strings.Contains(line, "up") {
			return "UP", nil
		} else if strings.Contains(line, "down") {
			return "DOWN", nil
		}
	}
	return "", fmt.Errorf("status not found in output")
}
