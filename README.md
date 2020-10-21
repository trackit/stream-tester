# README #

Stream tester

### High level design
* Golang test
    * Creates influencer
    * Pre-creates (some) fans, follows influencer
    * Starts stream
    * Starts spinning up fans that watch influencer's stream (either in process or over SSH remotely)
    * Collects CSV log lines that show stream progress
* Python visualizer
    * Reads CSV log lines and outputs a basic UI with bitrates and N stream lagged

### Setup dev environment ###

* `$ export GOPATH=/path/to/your/go/workspace`
* Clone this repository into the src folder of your GO workspace `git clone ... $GOPATH/src/trackit/stream-tester` (https://golang.org/doc/code.html#Overview)
* Build the project:
    ```
    $ cd $GOPATH/src/github.com/trackit/stream-tester
    $ go install
    ```
* Add dependencies to `vendor`
    ```
    $ govendor get github.com/trackit/stream-tester
    ```

### Build infrastructure ###

* Builds AMI using packer
    * Installs ffmpeg, rtmpdump, golang
    * Git exports latest or specified revision
    * Builds during AMI creation
    * Installs display script and others
* Builds infra using terraform, all nodes interchangeable

#### Install prerequisites

On mac OS X with homebrew
```
$ brew install terraform
$ brew install packer
$ brew install git
```

otherwise download and install from

https://www.terraform.io/intro/getting-started/install.html  
https://learn.hashicorp.com/tutorials/packer/getting-started-install  

and use your system's package management to install git

#### Creating infrastructure

```
$ cd ${GOPATH}/src/github.com/trackit/stream-tester/infra # or wherever you've unpacked this repo
$ cat config.json # adjust config.json for right number of instances etc.
{
    "aws_access_key": "AWS_ACCESS_KEY",
    "aws_secret_key": "AWS_SECRET_KEY",
    "aws_region": "AWS_REGION",
    "instance_type": "AWS_ISTANCE_TYPE",
    "number_instances": "AWS_NUMBER_OF_INSTANCES",
    "security_group_id": "AWS_SECURITY_GROUP_ID",
    "subnet_id": "AWS_SUBNET_ID",
    "source_ami": "AWS_AMI"
}
```

Now, you have to copy/paste your aws public/private ssh key into infra/files directory.  
And then execute **build.sh**:

```
$ ./build.sh
```

### Run test on built servers

```
$ terraform output
nodeip = [
    54.67.25.209,
    54.183.51.216,
    54.183.13.55
]
$ ssh -i files/id_rsa -l ubuntu 54.67.25.209 # any of the above IPs will work
$ runtest --help
Usage of stream_test runtest:
  -email string
        influencer email (default "trackit@trackit.io")
  -existingoffset int
        sequence number offset for existing users
  -percentnew int
        0-100 percentage of new fan users in test
  -precreatefans
        pre-create fans and follow influencer (not needed on repeat runs)
  -ramp duration
        time between users joining, e.g. 200ms (default 500ms)
  -sleepbetweensteps
  	Sleep between steps as a fan
  -sshhosts string
        space separated list of user@host to run test on (clustered)
  -sshkeyfile string
        path to SSH private key file
  -token string
        influencer token (default "4352915049.1677ed0.13fb746250c84b928b37360fba9e4d57")
  -timeout duration
    	Response time before timeout, e.g. 500ms
  -users int
        number of concurrent users (default 10)
  -videopath string
        path to video file used in test (default "640.flv")
/usr/local/bin/runtest: line 26:  3620 Terminated              tail -n +0 -f lastrun.stderr 1>&2
$ runtest --users 10 # runs test with 10 users
```

### Teardown infrastructure

```
$ cd ${GOPATH}/src/github.com/trackit/stream-tester/infra # or wherever you've unpacked this repo
$ ./destroy.sh
```
