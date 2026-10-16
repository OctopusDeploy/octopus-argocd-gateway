package configuration

import (
	"strings"

	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
	"github.com/spf13/viper"
)

type viperInstance interface {
	BindPFlag(key string, flag *pflag.Flag) error
	BindEnv(input ...string) error
}

type secretViper struct {
	viper.Viper
}

type ConfigManager struct {
	config  *viper.Viper
	secrets *secretViper
}

func NewConfigManager() *ConfigManager {
	return &ConfigManager{
		config:  createNewViper(),
		secrets: createNewSecretViper(),
	}
}

func createNewViper() *viper.Viper {
	instance := viper.New()
	instance.SetEnvKeyReplacer(strings.NewReplacer("-", "_"))
	instance.SetEnvPrefix("OCTOPUS")
	instance.AutomaticEnv() // read in environment variables that match2

	return instance
}

func createNewSecretViper() *secretViper {
	instance := createNewViper()
	return &secretViper{*instance}
}

// AddBoolFlag Adds a boolean flag to the provided flagSet and binds it to viper both as a flag and an environment variable
// Example: Adding a flag "debug" to the command line would result in both the --debug flag OCTOPUS_DEBUG environment variable and debug config file key been set and read
func (m *ConfigManager) AddBoolFlag(flagSet *pflag.FlagSet, name string, defaultValue bool, usage string) {
	flagSet.Bool(name, defaultValue, usage)
	err := m.config.BindPFlag(name, flagSet.Lookup(name))
	cobra.CheckErr(err)

	err = m.config.BindEnv(name)
	cobra.CheckErr(err)
}

// AddStringFlag Adds a string flag to the provided flagSet and binds it to viper both as a flag and an environment variable
// Example: Adding a flag "server-url" to the command line would result in both the --server-url flag OCTOPUS_SERVER_URL environment variable and server-url config file key been set and read
func (m *ConfigManager) AddStringFlag(flagSet *pflag.FlagSet, name string, defaultValue string, usage string) {
	addStringFlag(m.config, flagSet, name, defaultValue, usage)
}

// AddSecretStringFlag Adds a string flag to the provided flagSet and binds it to viper both as a flag and an environment variable
// Example: Adding a flag "server-url" to the command line would result in both the --server-url flag OCTOPUS_SERVER_URL environment variable and server-url config file key been set and read
func (m *ConfigManager) AddSecretStringFlag(flagSet *pflag.FlagSet, name string, defaultValue string, usage string) {
	addStringFlag(m.secrets, flagSet, name, defaultValue, usage)
}

func addStringFlag[T viperInstance](
	viperInstance T,
	flagSet *pflag.FlagSet,
	name string,
	defaultValue string,
	usage string,
) {
	flagSet.String(name, defaultValue, usage)
	err := viperInstance.BindPFlag(name, flagSet.Lookup(name))
	cobra.CheckErr(err)

	err = viperInstance.BindEnv(name)
	cobra.CheckErr(err)
}

// AddStringArrayFlag Adds a string array flag to the provided flagSet and binds it to viper both as a flag and an environment variable
// Example: adding a flag
func (m *ConfigManager) AddStringArrayFlag(flagSet *pflag.FlagSet, name string, defaultValue []string, usage string) {
	flagSet.StringArray(name, defaultValue, usage)
	err := m.config.BindPFlag(name, flagSet.Lookup(name))
	cobra.CheckErr(err)

	err = m.config.BindEnv(name)
	cobra.CheckErr(err)
}

// AddIntFlag Adds an integer flag to the provided flagSet and binds it to viper both as a flag and an environment variable
func (m *ConfigManager) AddIntFlag(flagSet *pflag.FlagSet, name string, defaultValue int, usage string) {
	flagSet.Int(name, defaultValue, usage)
	err := m.config.BindPFlag(name, flagSet.Lookup(name))
	cobra.CheckErr(err)

	err = m.config.BindEnv(name)
	cobra.CheckErr(err)
}

// AddGlobalFlags Called from the root cmd to setup all the possible global flags
func (m *ConfigManager) AddGlobalFlags(flagSet *pflag.FlagSet) {
	m.AddBoolFlag(flagSet, "debug", false, "Enable debug logging")
	m.AddStringFlag(flagSet, "gateway-certificate-path", "localcerts", "Path to load additional TLS certificates")
}
