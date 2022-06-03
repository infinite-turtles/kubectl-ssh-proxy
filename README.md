# kubectl ssh-proxy

Proxy OpenSSH client tools through Kubernetes pod

## Usage
```
	use = "kubectl ssh-proxy [flags] ssh|scp|sftp [flags] [arguments]"

	proxyExample = `
	# ssh login to remote system
	kubectl ssh-proxy ssh user@hostname

	# scp secure file copy
	kubectl ssh-proxy scp localpath [user@]host:[path]

	# sftp secure file transfer
	kubectl ssh-proxy sftp [user@]host[:path]
```