package cmd

import (
	"fmt"
	"github.com/hashicorp/go-checkpoint"
	"github.com/jsiebens/hashi-up/pkg/config"
	"github.com/jsiebens/hashi-up/pkg/operator"
	"github.com/pkg/errors"
	"github.com/spf13/cobra"
	"github.com/thanhpk/randstr"
	"net"
)

func InstallConsulCommand() *cobra.Command {

	var ip net.IP
	var user string
	var sshKey string
	var sshPort int
	var local bool
	var show bool
	var binary string

	var version string
	var datacenter string
	var bind string
	var advertise string
	var client string
	var server bool
	var boostrapExpect int64
	var retryJoin []string
	var encrypt string
	var caFile string
	var certFile string
	var keyFile string
	var enableConnect bool
	var enableACL bool
	var agentToken string

	var command = &cobra.Command{
		Use:          "install",
		SilenceUsage: true,
	}

	command.Flags().IPVar(&ip, "ip", net.ParseIP("127.0.0.1"), "Public IP of node")
	command.Flags().StringVar(&user, "user", "root", "Username for SSH login")
	command.Flags().StringVar(&sshKey, "ssh-key", "~/.ssh/id_rsa", "The ssh key to use for remote login")
	command.Flags().IntVar(&sshPort, "ssh-port", 22, "The port on which to connect for ssh")
	command.Flags().BoolVar(&local, "local", false, "Running the installation locally, without ssh")
	command.Flags().BoolVar(&show, "show", false, "Just show the generated config instead of deploying Consul")
	command.Flags().StringVar(&binary, "binary", "", "Upload and use this Nomad binary instead of downloading")

	command.Flags().StringVar(&version, "version", "", "Version of Consul to install, default to latest available")
	command.Flags().BoolVar(&server, "server", false, "Consul: switches agent to server mode. (see Consul documentation for more info)")
	command.Flags().StringVar(&datacenter, "datacenter", "dc1", "Consul: specifies the data center of the local agent. (see Consul documentation for more info)")
	command.Flags().StringVar(&bind, "bind", "", "Consul: sets the bind address for cluster communication. (see Consul documentation for more info)")
	command.Flags().StringVar(&advertise, "advertise", "", "Consul: sets the advertise address to use. (see Consul documentation for more info)")
	command.Flags().StringVar(&client, "client", "", "Consul: sets the address to bind for client access. (see Consul documentation for more info)")
	command.Flags().Int64Var(&boostrapExpect, "bootstrap-expect", 1, "Consul: sets server to expect bootstrap mode. (see Consul documentation for more info)")
	command.Flags().StringArrayVar(&retryJoin, "retry-join", []string{}, "Consul: address of an agent to join at start time with retries enabled. Can be specified multiple times. (see Consul documentation for more info)")
	command.Flags().StringVar(&encrypt, "encrypt", "", "Consul: provides the gossip encryption key. (see Consul documentation for more info)")
	command.Flags().StringVar(&caFile, "ca-file", "", "Consul: the certificate authority used to check the authenticity of client and server connections. (see Consul documentation for more info)")
	command.Flags().StringVar(&certFile, "cert-file", "", "Consul: the certificate to verify the agent's authenticity. (see Consul documentation for more info)")
	command.Flags().StringVar(&keyFile, "key-file", "", "Consul: the key used with the certificate to verify the agent's authenticity. (see Consul documentation for more info)")
	command.Flags().BoolVar(&enableConnect, "connect", false, "Consul: enables the Connect feature on the agent. (see Consul documentation for more info)")
	command.Flags().BoolVar(&enableACL, "acl", false, "Consul: enables Consul ACL system. (see Consul documentation for more info)")
	command.Flags().StringVar(&agentToken, "agent-token", "", "Consul: the token that the agent will use for internal agent operations.. (see Consul documentation for more info)")

	command.RunE = func(command *cobra.Command, args []string) error {
		var enableTLS = false

		if len(caFile) != 0 && len(certFile) != 0 && len(keyFile) != 0 {
			enableTLS = true
		}

		if !enableTLS && (len(caFile) != 0 || len(certFile) != 0 || len(keyFile) != 0) {
			return fmt.Errorf("ca-file, cert-file and key-file are all required when enabling tls, at least on of them is missing")
		}

		consulConfig := config.NewConsulConfiguration(datacenter, bind, advertise, client, server, boostrapExpect, retryJoin, encrypt, enableTLS, enableACL, agentToken, enableConnect)

		if show {
			fmt.Println(consulConfig)
			return nil
		}

		if len(binary) == 0 && len(version) == 0 {
			updateParams := &checkpoint.CheckParams{
				Product: "consul",
				Version: "0.0.0",
				Force:   true,
			}

			check, err := checkpoint.Check(updateParams)

			if err != nil {
				return errors.Wrapf(err, "unable to get latest version number, define a version manually with the --version flag")
			}

			version = check.CurrentVersion
		}

		callback := func(op operator.CommandOperator) error {
			dir := "/tmp/consul-installation." + randstr.String(6)

			defer op.Execute("rm -rf " + dir)

			_, err := op.Execute("mkdir " + dir)
			if err != nil {
				return fmt.Errorf("error received during installation: %s", err)
			}

			if len(binary) != 0 {
				err = op.UploadFile(binary, dir+"/consul", "0755")
				if err != nil {
					return fmt.Errorf("error received during upload consul binary: %s", err)
				}
			}

			if enableTLS {
				err = op.UploadFile(caFile, dir+"/consul-agent-ca.pem", "0640")
				if err != nil {
					return fmt.Errorf("error received during upload consul ca file: %s", err)
				}

				err = op.UploadFile(certFile, dir+"/consul-agent-cert.pem", "0640")
				if err != nil {
					return fmt.Errorf("error received during upload consul cert file: %s", err)
				}

				err = op.UploadFile(keyFile, dir+"/consul-agent-key.pem", "0640")
				if err != nil {
					return fmt.Errorf("error received during upload consul key file: %s", err)
				}
			}

			err = op.Upload(consulConfig, dir+"/consul.hcl", "0640")
			if err != nil {
				return fmt.Errorf("error received during upload consul configuration: %s", err)
			}

		err = op.Upload(InstallConsulScript, dir+"/install.sh", "0755")
		if err != nil {
			return fmt.Errorf("error received during upload install script: %s", err)
		}

			var serviceType = "notify"
			if len(retryJoin) == 0 {
				serviceType = "exec"
			}

			_, err = op.Execute(fmt.Sprintf("cat %s/install.sh | TMP_DIR='%s' SERVICE_TYPE='%s' CONSUL_VERSION='%s' sh -\n", dir, dir, serviceType, version))
			if err != nil {
				return fmt.Errorf("error received during installation: %s", err)
			}

			return nil
		}

		if local {
			return operator.ExecuteLocal(callback)
		} else {
			return operator.ExecuteRemote(ip, user, sshKey, sshPort, callback)
		}
	}

	return command
}

const InstallConsulScript = `
#!/bin/bash
set -e

info() {
  echo '[INFO] ' "$@"
}

fatal() {
  echo '[ERROR] ' "$@"
  exit 1
}

verify_system() {
  if ! [ -d /run/systemd ]; then
    fatal 'Can not find systemd to use as a process supervisor for consul'
  fi
}

setup_env() {
  SUDO=sudo
  if [ "$(id -u)" -eq 0 ]; then
    SUDO=
  fi

  CONSUL_DATA_DIR=/opt/consul
  CONSUL_CONFIG_DIR=/etc/consul.d
  CONSUL_CONFIG_FILE=/etc/consul.d/consul.hcl
  CONSUL_SERVICE_FILE=/etc/systemd/system/consul.service  
  
  BIN_DIR=/usr/local/bin

  PRE_INSTALL_HASHES=$(get_installed_hashes)
}

# --- set arch and suffix, fatal if architecture not supported ---
setup_verify_arch() {
  if [ -z "$ARCH" ]; then
    ARCH=$(uname -m)
  fi
  case $ARCH in
  amd64)
    SUFFIX=amd64
    ;;
  x86_64)
    SUFFIX=amd64
    ;;
  arm64)
    SUFFIX=arm64
    ;;
  aarch64)
    SUFFIX=arm64
    ;;
  arm*)
    SUFFIX=armhfv6
    ;;
  *)
    fatal "Unsupported architecture $ARCH"
    ;;
  esac
}

# --- get hashes of the current k3s bin and service files
get_installed_hashes() {
    $SUDO sha256sum ${BIN_DIR}/consul /etc/consul.d/consul.hcl /etc/consul.d/consul-agent-ca.pem /etc/consul.d/consul-agent-cert.pem /etc/consul.d/consul-agent-key.pem ${FILE_CONSUL_SERVICE} 2>&1 || true
}

has_yum() {
  [ -n "$(command -v yum)" ]
}

has_apt_get() {
  [ -n "$(command -v apt-get)" ]
}

install_dependencies() {
  if [ ! -x "${TMP_DIR}/consul" ]; then
    if ! [ -x "$(command -v unzip)" ] || ! [ -x "$(command -v curl)" ]; then
	    if $(has_apt_get); then
	  	  $SUDO apt-get update -y
	  	  $SUDO apt-get install -y curl unzip
	    elif $(has_yum); then
		  $SUDO yum update -y
		  $SUDO yum install -y curl unzip
	    else
		  fatal "Could not find apt-get or yum. Cannot install dependencies on this OS."
		  exit 1
	    fi
    fi
  fi
}

download_and_install() {
  if [ -x "${TMP_DIR}/consul" ]; then 
	info "Installing uploaded Consul binary"
	$SUDO cp "${TMP_DIR}/consul" $BIN_DIR
  else
    if [ -x "${BIN_DIR}/consul" ] && [ "$(${BIN_DIR}/consul version | grep Consul | cut -d' ' -f2)" = "v${CONSUL_VERSION}" ]; then
      info "Consul binary already installed in ${BIN_DIR}, skipping downloading and installing binary"
    else
      info "Downloading and unpacking consul_${CONSUL_VERSION}_linux_${SUFFIX}.zip"
	  curl -o "$TMP_DIR/consul.zip" -sfL "https://releases.hashicorp.com/consul/${CONSUL_VERSION}/consul_${CONSUL_VERSION}_linux_${SUFFIX}.zip"
      $SUDO unzip -qq -o "$TMP_DIR/consul.zip" -d $BIN_DIR
    fi
  fi
}

create_user_and_config() {
  if $(id consul >/dev/null 2>&1); then
    info "User consul already exists. Will not create again."
  else
    info "Creating user named consul"
    $SUDO useradd --system --home ${CONSUL_CONFIG_DIR} --shell /bin/false consul
  fi

  $SUDO mkdir --parents ${CONSUL_DATA_DIR}
  $SUDO mkdir --parents ${CONSUL_CONFIG_DIR}

  $SUDO cp "${TMP_DIR}/consul.hcl" ${CONSUL_CONFIG_FILE}
  if [ -f "${TMP_DIR}/consul-agent-ca.pem" ]; then
	$SUDO cp "${TMP_DIR}/consul-agent-ca.pem" /etc/consul.d/consul-agent-ca.pem
  fi
  if [ -f "${TMP_DIR}/consul-agent-cert.pem" ]; then
	$SUDO cp "${TMP_DIR}/consul-agent-cert.pem" /etc/consul.d/consul-agent-cert.pem
  fi
  if [ -f "${TMP_DIR}/consul-agent-key.pem" ]; then
	$SUDO cp "${TMP_DIR}/consul-agent-key.pem" /etc/consul.d/consul-agent-key.pem
  fi

  $SUDO chown --recursive consul:consul /opt/consul
  $SUDO chown --recursive consul:consul /etc/consul.d
}

# --- write systemd service file ---
create_systemd_service_file() {
  info "Creating service file ${CONSUL_SERVICE_FILE}"
  $SUDO tee ${CONSUL_SERVICE_FILE} >/dev/null <<EOF
[Unit]
Description="HashiCorp Consul - A service mesh solution"
Documentation=https://www.consul.io/
Requires=network-online.target
After=network-online.target
ConditionFileNotEmpty=/etc/consul.d/consul.hcl

[Service]
Type=${SERVICE_TYPE}
User=consul
Group=consul
ExecStart=${BIN_DIR}/consul agent -config-dir=${CONSUL_CONFIG_DIR}
ExecReload=${BIN_DIR}/consul reload
ExecStop=${BIN_DIR}/consul leave
KillMode=process
Restart=on-failure
LimitNOFILE=65536

[Install]
WantedBy=multi-user.target
EOF
}

# --- startup systemd or openrc service ---
systemd_enable_and_start() {
 	info "Enabling consul unit"
  	$SUDO systemctl enable ${CONSUL_SERVICE_FILE} >/dev/null
  	$SUDO systemctl daemon-reload >/dev/null
    
	POST_INSTALL_HASHES=$(get_installed_hashes)
    if [ "${PRE_INSTALL_HASHES}" = "${POST_INSTALL_HASHES}" ]; then
        info 'No change detected so skipping service start'
        return
    fi

  	info "Starting consul"
  	$SUDO systemctl restart consul

    return 0
}

setup_env
setup_verify_arch
verify_system
install_dependencies
create_user_and_config
download_and_install
create_systemd_service_file
systemd_enable_and_start

`
