package main

import (
	"encoding/csv"
	"flag"
	"fmt"
	"log"
	"os"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"github.com/trackit/stream-tester/client"
	"github.com/trackit/stream-tester/remote"
	"github.com/trackit/stream-tester/rtmp"
	"github.com/trackit/stream-tester/usergenerator"
	"github.com/trackit/stream-tester/instanceinfos"
)

func main() {
	bumpNoFiles(8192)
	var concurrentUsers int
	var rampUpTime time.Duration
	var apiTimeout time.Duration
	var existingUserOffset int
	var percentNewUsers int
	var sshHosts string
	var sshPKPath string
	var influencerID int
	var influencerEmail string
	var influencerToken string
	var testVideoPath string
	var precreateFans bool
	var sleepBetweenSteps bool
	var apiBaseURL string
	var streamsAPIBaseURL string
	var streamsAPIToken string
	commandName := "runtest"
	args := os.Args[1:]
	if len(os.Args) >= 2 && len(os.Args[1]) > 0 && os.Args[1][0] != '-' {
		commandName = os.Args[1]
		args = os.Args[2:]
	}
	f := flag.NewFlagSet(os.Args[0]+" "+commandName, flag.ContinueOnError)
	f.IntVar(&concurrentUsers, "users", 10, "number of concurrent users")
	f.IntVar(&existingUserOffset, "existingoffset", 0, "sequence number offset for existing users")
	f.IntVar(&percentNewUsers, "percentnew", 0, "0-100 percentage of new fan users in test")
	f.DurationVar(&rampUpTime, "ramp", 500*time.Millisecond, "time between users joining, e.g. 200ms")
	f.DurationVar(&apiTimeout, "timeout", 0*time.Millisecond, "Response time before timeout, e.g. 500ms")
	f.BoolVar(&sleepBetweenSteps, "sleepbetweensteps", false, "Sleep between steps as a fan")
	f.StringVar(&apiBaseURL, "apiurl", "", "API url base")
	f.StringVar(&streamsAPIBaseURL, "streamsapiurl", "", "Streams API url base")
	f.StringVar(&streamsAPIToken, "streamsapitoken", "", "Streams API token")
	switch commandName {
	case "runtest":
		f.StringVar(&sshHosts, "sshhosts", "", "space separated list of user@host to run test on (clustered)")
		f.StringVar(&sshPKPath, "sshkeyfile", "", "path to SSH private key file")
		f.StringVar(&influencerEmail, "email", "trackit@trackit.io", "influencer email")
		f.StringVar(&influencerToken, "token", "4352915049.1677ed0.13fb746250c84b928b37360fba9e4d57", "influencer token")
		f.StringVar(&testVideoPath, "videopath", "640.flv", "path to video file used in test")
		f.BoolVar(&precreateFans, "precreatefans", false, "pre-create fans and follow influencer (not needed on repeat runs)")
		break
	case "runfans":
		f.IntVar(&influencerID, "influencerid", 0, "influencer id to have fans join")
		break
	default:
		os.Stderr.WriteString("Unknown command " + commandName + "\nOptions:\n")
		os.Stderr.WriteString(" runtest (default)\trun full test, possibly remotely\n")
		os.Stderr.WriteString(" runfans        \tonly runs the fan portion, used by remote\n")
		os.Exit(2)
	}

	err := f.Parse(args)
	if err != nil {
		os.Exit(2)
	}
	fanClient = client.NewFanClient(apiTimeout)
	if apiBaseURL != "" {
		fanClient.BaseURL = apiBaseURL
		influencerClient.BaseURL = apiBaseURL
	}
	if streamsAPIBaseURL != "" {
		fanClient.StreamsBaseUrl = streamsAPIBaseURL
	}
	if streamsAPIToken != "" {
		fanClient.StreamsToken = streamsAPIToken
	}
	userGenerator, err = usergenerator.NewUserGenerator(int32(existingUserOffset), int32(percentNewUsers))
	if err != nil {
		log.Fatal(err)
	}

	machineID, err := os.Hostname()
	if err != nil {
		log.Fatal(err)
	}
	if machineID == "" {
		machineID = "instance-" + usergenerator.RandomString(5)
	}

	switch commandName {
	case "runtest":
		if sshHosts == "" {
			runInfluencer(influencerEmail, influencerToken, testVideoPath, precreateFans, concurrentUsers, percentNewUsers, func(influencerID int) {
				runFans(concurrentUsers, rampUpTime, sleepBetweenSteps, existingUserOffset, influencerID, machineID, csvWriter())
			})
		} else {
			remote, err := remote.NewRemote(sshHosts, sshPKPath)
			if err != nil {
				log.Fatal(err)
			}
			log.Println("Connecting to remotes ", sshHosts)
			err = remote.Connect()
			if err != nil {
				log.Fatal(err)
			}
			runInfluencer(influencerEmail, influencerToken, testVideoPath, precreateFans, concurrentUsers, percentNewUsers, func(influencerID int) {
				concurrentUsersPerNode := concurrentUsers / len(remote.Nodes)
				rampUpTimePerNode := rampUpTime * time.Duration(len(remote.Nodes))
				commandString := "stream_test runfans"
				commandString += fmt.Sprintf(" -influencerid %d", influencerID)
				commandString += fmt.Sprintf(" -users %d", concurrentUsersPerNode)
				commandString += fmt.Sprintf(" -ramp %v", rampUpTimePerNode.String())
				commandString += fmt.Sprintf(" -percentnew %d", percentNewUsers)
				if apiBaseURL != "" {
					commandString += fmt.Sprintf(" -apiurl %s", apiBaseURL)
				}
				if streamsAPIBaseURL != "" {
					commandString += fmt.Sprintf(" -streamsapiurl %s", streamsAPIBaseURL)
				}
				if streamsAPIToken != "" {
					commandString += fmt.Sprintf(" -streamsapitoken %s", streamsAPIToken)
				}
				i := 0
				remote.StartEach(func() (string, error) {
					offset := existingUserOffset + concurrentUsers*2*i
					i++
					return commandString + fmt.Sprintf(" -existingoffset %d", offset), nil
				})
			})
		}
		break
	case "runfans":
		runFans(concurrentUsers, rampUpTime, sleepBetweenSteps, existingUserOffset, influencerID, machineID, csvWriter())
		break
	}
}

func runInfluencer(email, token, testVideoPath string, shouldPrecreatFans bool, concurrentUsers, percentNewUsers int, run func(int)) {
	influencerCreds := signInInfluencer(email, token)

	if shouldPrecreatFans {
		precreateFans(influencerCreds.ID, concurrentUsers*(100-percentNewUsers)/100)
	}

	influencer := startInfluencer(influencerCreds)

	originURL := client.GetOriginUrl(influencer.ServerStatus.OriginIP, influencer.Username)
	log.Println("Pushing to", originURL)
	pusher := rtmp.NewRTMPPusher(originURL, testVideoPath)

	go func() {
		log.Println("Waiting 5 seconds to start fans")
		time.Sleep(5 * time.Second)
		run(influencer.ID)
	}()
	err := pusher.Run()
	if err != nil {
		log.Fatal("Error pushing to ", originURL, " ", err)
	}
}

func precreateFans(influencerID int, nUsers int) {
	log.Printf("Pre-creating %d users and following influencer (this may take a while)\n", nUsers)
	for _, fanUsername := range userGenerator.GetExisting(nUsers) {
		fanRes, err := fanClient.SignIn(fanUsername+"@e.com", password)
		if err != nil {
			log.Println("Fan", fanUsername, "signin failure", err)
			fanRes, err = fanClient.SignUp(fanUsername+"@e.com", fanUsername, password)
			if err != nil {
				log.Println("Fan", fanUsername, "signup failure", err)
			}
			log.Println("Fan", fanUsername, "signed up")
		} else {
			log.Println("Fan", fanUsername, "signed in")
		}
		if err = fanClient.FollowInfluencer(fanRes.Token, influencerID); err != nil {
			log.Println("Fan", fanUsername, "follow failure", err)
		}
		log.Println("Fan", fanUsername, "followed influencer")
	}
}

func fanRequestMarketplace(fanUsername, machineID string, fanRes *client.FanResponse, sleepBetweenSteps bool, startTime time.Time, out chan []string) error {
	generalMarketplaceResp, err := fanClient.GetGeneralMarketplace(fanRes.Token)
	printApiRequestCSV(fanUsername, machineID, startTime, out)
	if err != nil {
		logErrorAPI("Fan", fanUsername, "general influencers marketplace error", err, "warning", startTime, out)
		return err
	}
	log.Println("Fan", fanUsername, "requested general marketplace")
	influencersLen := len(generalMarketplaceResp.Influencers)
	ids := make([]int, influencersLen, influencersLen)
	for index, element := range generalMarketplaceResp.Influencers {
		ids[index] = element.ID
	}
	err = fanClient.RelationMarketplace(fanRes.Token, ids)
	printApiRequestCSV(fanUsername, machineID, startTime, out)
	if err != nil {
		logErrorAPI("Fan", fanUsername, "relation marketplace error", err, "warning", startTime, out)
		return err
	}
	log.Println("Fan", fanUsername, "requested relation marketplace")
	if sleepBetweenSteps {
		time.Sleep(5 * time.Second)
	}
	return nil
}

func fanSignUpAndFollow(fanUsername, machineID string, influencerID int, sleepBetweenSteps bool, startTime time.Time, out chan []string) (*client.FanResponse, error) {
	fanRes, err := fanClient.SignUp(fanUsername+"@e.com", fanUsername, password)
	printApiRequestCSV(fanUsername, machineID, startTime, out)
	if err != nil {
		logErrorAPI("Fan", fanUsername, "signup failure", err, "critical", startTime, out)
		return nil, err
	}
	log.Println("Fan", fanUsername, "signed up")
	if sleepBetweenSteps {
		time.Sleep(3 * time.Second)
	}

	err = fanClient.UseCode(fanRes.Token)
	printApiRequestCSV(fanUsername, machineID, startTime, out)
	if err != nil {
		logErrorAPI("Fan", fanUsername, "Use code failure", err, "warning", startTime, out)
	}
	log.Println("Fan", fanUsername, "Use code")
	if sleepBetweenSteps {
		time.Sleep(5 * time.Second)
	}
	fanRequestMarketplace(fanUsername, machineID, fanRes, sleepBetweenSteps, startTime, out)

	err = fanClient.FollowInfluencer(fanRes.Token, influencerID)
	printApiRequestCSV(fanUsername, machineID, startTime, out)
	if err != nil {
		logErrorAPI("Fan", fanUsername, "follow influencer failure", err, "critical", startTime, out)
	}
	log.Println("Fan", fanUsername, "followed influencer")
	if sleepBetweenSteps {
		time.Sleep(1 * time.Second)
	}
	return fanRes, err
}

func runFans(concurrentUsers int, rampUpTime time.Duration, sleepBetweenSteps bool, existingUserOffset int, influencerID int, machineID string, out chan []string) {
	startTimeInstanceInfos := time.Now()
	go func() {
		for {
			bytesSent, bytesRecv := instanceinfos.GetNetIOBytes()
			out <- []string{
				machineID,
				strconv.FormatFloat(secsSince(startTimeInstanceInfos), 'f', 2, 32),
				"KiloBytesSent",
				strconv.FormatUint(bytesSent / 1024, 10)}
			out <- []string{
				machineID,
				strconv.FormatFloat(secsSince(startTimeInstanceInfos), 'f', 2, 32),
				"KiloBytesRecv",
				strconv.FormatUint(bytesRecv / 1024, 10)}

			cpuUsage := instanceinfos.GetCPUUsage()
			out <- []string{
				machineID,
				strconv.FormatFloat(secsSince(startTimeInstanceInfos), 'f', 2, 32),
				"CPUUsage",
				strconv.FormatFloat(float64(cpuUsage), 'f', 2, 32)}

			time.Sleep(2 * time.Second)
		}
	}()
	runN(concurrentUsers, rampUpTime, func(_ int) {
		startTime := time.Now()

		fanUsername, newUser := userGenerator.Gen()
		log.Println("Starting client", fanUsername)
		printStartTestCSV(fanUsername, machineID, out)

		var fanRes *client.FanResponse
		var err error
		if newUser {
			fanRes, err = fanSignUpAndFollow(fanUsername, machineID, influencerID, sleepBetweenSteps, startTime, out)
			if err != nil {
				return
			}
		} else {
			fanRes, err = fanClient.SignIn(fanUsername+"@e.com", password)
			printApiRequestCSV(fanUsername, machineID, startTime, out)
			if err != nil {
				logErrorAPI("Fan", fanUsername, "signin failure", err, "warning", startTime, out)
				fanRes, err = fanSignUpAndFollow(fanUsername, machineID, influencerID, sleepBetweenSteps, startTime, out)
				if err != nil {
					return
				}
			} else {
				log.Println("Fan", fanUsername, "signed in")
			}
			fanRequestMarketplace(fanUsername, machineID, fanRes, sleepBetweenSteps, startTime, out)
		}

		joined, err := fanClient.JoinStream(influencerID, fanRes.ID)
		printApiRequestCSV(fanUsername, machineID, startTime, out)
		if err != nil {
			logErrorAPI("Fan", fanUsername, "join failure", err, "critical", startTime, out)
			return
		}
		log.Println("Fan", fanUsername, "joined stream")

		err = fanClient.LeaveStream(influencerID, fanRes.ID)
		printApiRequestCSV(fanUsername, machineID, startTime, out)
		if err != nil {
			logErrorAPI("Fan", fanUsername, "leave failure", err, "critical", startTime, out)
			return
		}
		log.Println("Fan", fanUsername, "left stream")

		rtmpUrl, err := client.GetEdgeUrl(joined.OriginIP, joined.InfluencerUsername)
		if err != nil {
			logErrorAPI("Fan", fanUsername, "leave failure", err, "critical", startTime, out)
			return
		}

		log.Println("Connecting client", fanUsername, "to", rtmpUrl)
		test := rtmp.NewRTMPTest(rtmpUrl)
		go func() {
			for prog := range test.Progress {
				out <- []string{
					fanUsername,
					strconv.FormatFloat(secsSince(startTime), 'f', 2, 32),
					"StreamProgressKiloBytes",
					strconv.FormatFloat(float64(prog.KiloBytes), 'f', 2, 32)}
				out <- []string{
					fanUsername,
					strconv.FormatFloat(secsSince(startTime), 'f', 2, 32),
					"StreamProgressSeconds",
					strconv.FormatFloat(float64(prog.Seconds), 'f', 2, 32)}
			}
		}()
		err = test.Run()
		if err != nil {
			log.Println(err)
		}
	})
}

func signInInfluencer(email, token string) *client.InfluencerResponse {
	log.Println("SignIn as influencer", email)
	infCreds, err := influencerClient.InstagramSignInOrUp(email, token)
	if err != nil {
		log.Fatal("Failed to sign in", err)
	}
	return infCreds
}

func startInfluencer(infCreds *client.InfluencerResponse) (inf *client.InfluencerResponse) {
	var err error
	log.Println("Creating stream alerts")
	if err = influencerClient.CreateStreamAlerts(infCreds.ID, infCreds.Token); err != nil {
		log.Fatal(err)
	}

	for {
		log.Println("Polling influencer for readiness")
		inf, err = influencerClient.Get(infCreds.ID, infCreds.Token)
		if err != nil {
			log.Fatal(err)
		}
		if inf.ServerStatus.Ready {
			log.Println("Influencer ready")
			break
		}
		time.Sleep(5 * time.Second)
	}

	log.Println("Creating stream status")
	if err := influencerClient.CreateStream(infCreds.ID, infCreds.Token); err != nil {
		log.Fatal(err)
	}
	go func() {
		for {
			time.Sleep(5 * time.Minute)
			log.Println("Creating stream status")
			if err := influencerClient.CreateStream(infCreds.ID, infCreds.Token); err != nil {
				log.Fatal(err)
			}
		}
	}()
	return
}

func runN(count int, rampUpTime time.Duration, body func(int)) {
	var wait sync.WaitGroup
	queue := make(chan struct{}, count)
	i := 0
	for {
		time.Sleep(rampUpTime)
		wait.Add(1)
		queue <- struct{}{}
		go func() {
			defer func() {
				wait.Done()
				<-queue
			}()
			body(i)
		}()
		i++
	}
	wait.Wait()
}

func csvWriter() chan []string {
	out := make(chan []string)
	go func() {
		csvWriter := csv.NewWriter(os.Stdout)
		for row := range out {
			err := csvWriter.Write(row)
			csvWriter.Flush()
			if err != nil {
				log.Println(err)
			}
		}
	}()
	return out
}

func secsSince(t time.Time) float64 {
	return float64(time.Since(t)/time.Millisecond) / 1000
}

func bumpNoFiles(noFiles uint64) error {
	var rlim syscall.Rlimit
	err := syscall.Getrlimit(syscall.RLIMIT_NOFILE, &rlim)
	if err != nil {
		return err
	}
	if rlim.Cur < noFiles {
		rlim.Cur = noFiles
	}
	return syscall.Setrlimit(syscall.RLIMIT_NOFILE, &rlim)
}

func logErrorAPI(userType, fanUsername, message string, err error, level string, startTime time.Time, out chan []string) {
	log.Println(userType, fanUsername, message, err)
	if strings.Contains(err.Error(), "Timeout") {
		out <- []string{
			fanUsername,
			strconv.FormatFloat(secsSince(startTime), 'f', 2, 32),
			"ApiRequestTimeout",
			level}
	} else {
		out <- []string{
			fanUsername,
			strconv.FormatFloat(secsSince(startTime), 'f', 2, 32),
			"ApiError",
			level}
	}
}

func printApiRequestCSV(fanUsername, machineID string, startTime time.Time, out chan []string) {
	out <- []string{
		fanUsername,
		strconv.FormatFloat(secsSince(startTime), 'f', 2, 32),
		"ApiRequest",
		machineID}
}

func printStartTestCSV(fanUsername string, machineID string, out chan []string) {
	out <- []string{
		fanUsername,
		"0",
		"StartTestOnMachine",
		machineID}
}

var fanClient *client.FanClient

var influencerClient = client.NewInfluencerClient()

var userGenerator *usergenerator.UserGenerator

var password = "Password42"
