package constant

const (
	// DefaultKeyName is the default name for the SSH key file
	DefaultKeyName = "nvair.unifabric.io"

	// DefaultUbuntuUser is the default SSH username for Ubuntu-based hosts
	DefaultUbuntuUser = "ubuntu"

	// DefaultUbuntuPassword is the default SSH password for Ubuntu-based hosts
	DefaultUbuntuPassword = "nvidia"

	// DefaultCumulusUser is the default SSH username for Cumulus Linux switches
	DefaultCumulusUser = "cumulus"

	// DefaultCumulusOldPassword is the default factory SSH password for Cumulus Linux switches
	// This password in plain text is safe. It is only used to skip the password reset step. All connections are made using SSH key files.
	DefaultCumulusOldPassword = "cumulus"

	// DefaultCumulusNewPassword is the new SSH password for Cumulus Linux switches after reset
	// This password in plain text is safe. It is only used to skip the password reset step. All connections are made using SSH key files.
	DefaultCumulusNewPassword = "Dangerous1#"

	// DefaultBastionUser is the default SSH username for the bastion host
	// This password in plain text is safe. It is only used to skip the password reset step. All connections are made using SSH key files.
	DefaultBastionNewPassword = "dangerous"
)
