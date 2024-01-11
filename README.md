# monotf

`monotf` is a tool for managing monorepo(s) of Terraform workspaces. `monotf` provides a centralized way to manage Terraform versions and to run Terraform commands across multiple workspaces. Upstream `terraform` requires the purchase of Terraform Enterprise / Cloud for central state summarization, management, and workspace queuing - `monotf` provides a free alternative which can be run in any workflow tool.

## Terraform & Monorepos

Terraform is a great tool for managing infrastructure as code. However, it is not designed to be used in a monorepo, as the number of resources grows it slows down the terraform execution. This is because terraform must load all of the resources in the monorepo before it can execute any commands. This is a known issue, and there are some workarounds, but they are not ideal.

However when centrally managing platform resources, a monorepo can be useful to centrally roll out changes across multiple environments. For example, if you have a platform which is deployed across multiple AWS accounts, you may want to make a change to the platform and roll it out to all of the accounts. A monorepo is a great way to do this, as you can make the change in one place and then roll it out to all of the accounts.

`monotf` aims to provide "the best of both worlds" - you can create a mono-repo structure, while isolating execution boundaries at specifed levels - and of course not having to pay for Terraform Enterprise.

## Terraform Backend

`monotf` does _not_ manage terraform state. Instead, you must configure a supported [backend](https://developer.hashicorp.com/terraform/language/settings/backends/configuration) for state management. `monotf` simply manages the workspace meta configuration.

As outlined in the Terraform documentation, it is not recommended to store sensitive configuration in the backend block. Instead, you should store your backend config (such as your `pg` connection string) in a secure location and make it available as an environment variable at runtime. This is discussed in the [Environment Variables](#environment-variables) section below.

## Usage

`monotf` is a command line tool. It is designed to be used in a CI/CD pipeline, but can also be used locally. It enables decentralized / distributed workflow execution (such as in GitHub Actions) while maintaining a centralized configuration and queueing system.

```
usage: monotf [flags] <command> [args]
  -addr string
        monotf server to use
  -config string
        path to config file (default "monotf.yaml")
  -dir string
        path to repo directory
  -init
        initialize repo (default true)
  -log-level string
        log level (default "debug")
  -port int
        port to run server on (default 8080)
  -vault-addr string
        vault address
  -vault-namespace string
        vault namespace
  -vault-path string
        vault path
  -w string
        workspace to use
  -wait string
        timeout for waiting for workspace to be ready. 0 means no timeout (default "0s")
commands:
  sys-init
  server
  terraform
  terraform-speculative-plan
  terraform-plan-apply
```

### Commands

#### `sys-init`

Initialize the system with the supported binaries. This can be called as part of a container build to pre-cache the binaries.

#### `server`

Run the `monotf` server. This is used to store workspace metadaata and provide a basic queueing system for workspace executions. Note that the server does not manage state, that is managed by the Terraform backend. Also note that the actual terraform code execution does not happen on the server (as it does with Terraform Enterprise), instead the server simply manages the queue and provides a way to execute the code in a distributed fashion.

#### `terraform`

Run a terraform command in a workspace. This command will queue the workspace and wait for it to be ready before executing the command.

#### `terraform-speculative-plan`

Run a speculative plan in a workspace. This command will queue the workspace and wait for it to be ready before executing the command. This command is useful for running a plan in a workspace without actually applying it. This can be used to check for errors in the code, or to check for drift in the infrastructure.

#### `terraform-plan-apply`

Run a plan and apply in a workspace. This command will queue the workspace and wait for it to be ready before executing the command. This command is useful for running as part of an auto-merge workflow, where you want to run a plan and apply in a workspace after a PR is merged.


## Configuration File

`monotf` is configured using a `yaml` file. The default location for this file is `./monotf.yaml`, but you can specify a different location using the `-config` flag.

Example configuration file:

```yaml
# directory in which to store terraform binaries
bin_dir: /tmp/bin
# supported terraform versions
versions:
- 1.5.7
- 1.6.6
# default terraform version if not specified in
# the workspace with a .terraform-version file
default_version: 1.6.6
# org name, useful when using a common remote state backend
# this will be used to create a unique workspace name
org: testing
# the monotf server address
server_addr: http://localhost:8080
# the directory in which the terraform code is stored
dir: "test"
# optional: infer environment variables from the path of the workspace
# for example, if you store your code in "test/aws13" then you can
# infer the AWS_PROFILE environment variable from the workspace path
path_template: "{{AWS_PROFILE}}"
# optional: a script which will be run in the workspace directory
# to set environment variables for the terraform shell
var_script: "$PWD/test/vars.sh"
# optional: retrieve environment variables for the terraform shell
# from a vault secret. For enterprise vault, you can specify a namespace
vault_env:
  addr: https://vault.example.com
  namespace: ""
  path: "kv/myapp/env"
```

## Terraform Workspace Name

The terraform workspace name is generated by combining the `org` with the workspace `path` relative to the `dir`, where slashes (`/`) are replaced with hyphens (`-`). For example, if you have the following directory structure:

```
my-repo-name
├── aws01
│   ├── us-east-1
│   │   └── main.tf
│   └── us-west-2
│       └── main.tf
├── aws02
│   ├── us-east-1
│   │   └── main.tf
│   └── us-west-2
│       └── main.tf
```

And you have the following configuration:

```yaml
org: my-org
dir: my-repo-name
```

Then the following workspaces will be created:

```
my-org-aws01-us-east-1
my-org-aws01-us-west-2
my-org-aws02-us-east-1
my-org-aws02-us-west-2
```

## Environment Variables

`monotf` makes uses of environment variables to configure the terraform workspace. The goal is to make your terraform structure as declarative as possible, and let the structure of the workspace determine the configuration. In addition to the existing system environment variables, `monotf` will use the following methods to set environment variables:

### Path Template

The `path_template` configuration option allows you to infer environment variables from the path of the workspace. For example, if you store your code in `my-repo-name/aws01/us-east-1` then you can infer the `AWS_PROFILE` and `AWS_REGION` environment variables from the workspace path.

For the example above, you would configure the following:

```yaml
path_template: "{{AWS_PROFILE}}/{{AWS_REGION}}"
```

Then, when you run `monotf` commands in the `aws01/us-east-1` workspace, the following environment variables will be set:

```
AWS_PROFILE=aws01
AWS_REGION=us-east-1
```

### Var Script

The `var_script` configuration option allows you to run a script in the workspace directory to set environment variables for the terraform shell. This is useful if you need to set environment variables which can be inferred from the workspace context.

The script can be written any language that can be run as an executable. A `MONOTF_VARS` environment variable will be available in the script's environment. The script can write any environment variables to `MONOTF_VARS` in the format `VAR=value`.

The script is run in the workspace directory, so you can use relative paths to access files in the workspace. Additionally, inferred environment variables will be available in the script's environment.

For example, assuming you set the above `path_template` configuration, you could use the following `var_script` to set a `TF_VAR_role_arn` variable which can be used to assume a defined role:

```bash
#!/bin/bash
account_number=$(aws sts get-caller-identity --profile $AWS_PROFILE --query Account --output text)
echo TF_VAR_role_arn=arn:aws:iam::$account_number:role/TerraformIAM >> $MONOTF_VARS
```

Note that it is using the `AWS_PROFILE` environment variable which was inferred from the workspace path to run a command to then set the `TF_VAR_role_arn` environment variable.

Note: make sure your script is executable (`chmod +x`).

### Vault Environment Variables

Assuming you are using a backend which requires some secret parameters to access, you will need to specify those as `TF_VAR_` environment variables. This can be done out of scope of the terraform, but it is sometimes difficult to remember / configure appropriately. If you store your variable(s) in HashiCorp Vault, you can use the `vault_env` configuration option to retrieve them automatically.

You will need to set the `VAULT_TOKEN` env var first - that is still out of scope of `monotf`. Then, you can configure `monotf` to retrieve the variables from Vault before running the terraform.

## Deployment

Monotf operates in a client/server model. The server is used to store workspace metadata and provide a basic queueing system for workspace executions. The client is used to execute terraform commands in the workspace.

Both client and server are available as precompiled binaries as well as docker images. The binaries are available on the [releases](https://github.com/robertlestak/monotf/releases) page. The docker images are available on [Docker Hub](https://hub.docker.com/r/robertlestak/monotf).

### Docker Image

The docker image available in DockerHub _only_ has the `monotf` binary, `curl`, `unzip`, `sh`, and a few other packages avaiable in the `alpine` base image. In practice you will probably find the need for additional binaries in your workflow, so you will probably want to create your own image based on the `monotf` image, or by downloading the precompiled binaries and adding them to your own image.

### Server

See `manifests` for example kubernetes yaml files for deploying the server. Note this only creates a `ClusterIP` service, you will need to expose it as appropriate for your environment. Make sure you fill in your secret values in the yaml files before deploying. Obviously at scale you would probably be using something like [external-secrets.io](https://external-secrets.io) to manage this.

#### Database Backend

The server relies on a database backend to store workspace metadata. Currently the server supports PostgreSQL and SQLite. The database backend is configured using environment variables. The following environment variables are supported:

| Variable | Description | Driver Support |
| --- | --- | --- |
| `DB_DRIVER` | The database driver to use. Currently supported: `sqlite`, `postgres` | All |
| `DB_PATH` | The path to the database file. | `sqlite` |
| `DB_HOST` | The database host. | `postgres` |
| `DB_PORT` | The database port. | `postgres` |
| `DB_USER` | The database user | `postgres` |
| `DB_PASS` | The database password | `postgres` |
| `DB_NAME` | The database name | `postgres` |

### Client

Once the server is up and running, you can configure your repo(s) to use the appropriate GitHub Actions (or similar workflow solution) to run the client. See the [.github/example-workflows](.github/example-workflows) directory for example workflows. These are intentionally minimal, you will probably want to copy / paste and modify them to suit your needs.