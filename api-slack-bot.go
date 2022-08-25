package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"regexp"
	"strings"

	"github.com/joho/godotenv"
	"github.com/slack-go/slack"
	"github.com/slack-go/slack/slackevents"
	"github.com/slack-go/slack/socketmode"
)

var hostsmap = map[string]string{
	"ServerGreen1": "ServerGreen1.domain.com",
	"ServerBlue1":  "ServerBlue1.domain.com",

	"ServerGreen2": "ServerGreen2.domain.com",
	"ServerBlue2":  "ServerBlue2.domain.com",

	"ServerGreen3": "ServerGreen3.domain.com",
	"ServerBlue3":  "ServerBlue3.domain.com",
}

var balancerSlice = map[string]string{
	"site1": "https://api1.domain.com/get",
	"site2": "https://api2.domain.com/get",
	"site3": "https://api3.domain.com/get",
}

type Balancer struct {
	Farms    []Farms     `json:"farms"`
	Hosts    interface{} `json:"hosts"`
	Nodes    interface{} `json:"nodes"`
	Response interface{} `json:"response"`
	Status   string      `json:"status"`
}
type Farms struct {
	Active  interface{} `json:"active"`
	Enabled string      `json:"enabled"`
	Name    string      `json:"name"`
	Nodes   interface{} `json:"nodes"`
}

type HostResult struct {
	Region     string
	ActiveNode string
	Build      string
}

func main() {

	// Load Env variables from .dot file
	godotenv.Load(".env")

	token := os.Getenv("SLACK_AUTH_TOKEN")
	appToken := os.Getenv("SLACK_APP_TOKEN")
	// Create a new client to slack by giving token
	// Set debug to true while developing
	// Also add a ApplicationToken option to the client
	client := slack.New(token, slack.OptionDebug(true), slack.OptionAppLevelToken(appToken))
	// go-slack comes with a SocketMode package that we need to use that accepts a Slack client and outputs a Socket mode client instead
	socket := socketmode.New(
		client,
		socketmode.OptionDebug(true),
		// Option to set a custom logger
		socketmode.OptionLog(log.New(os.Stdout, "socketmode: ", log.Lshortfile|log.LstdFlags)),
	)

	// Create a context that can be used to cancel goroutine
	ctx, cancel := context.WithCancel(context.Background())
	// Make this cancel called properly in a real program , graceful shutdown etc
	defer cancel()

	go func(ctx context.Context, client *slack.Client, socket *socketmode.Client) {
		// Create a for loop that selects either the context cancellation or the events incomming
		for {
			select {
			// inscase context cancel is called exit the goroutine
			case <-ctx.Done():
				log.Println("Shutting down socketmode listener")
				return
			case event := <-socket.Events:
				// We have a new Events, let's type switch the event
				// Add more use cases here if you want to listen to other events.
				switch event.Type {
				// handle EventAPI events
				case socketmode.EventTypeEventsAPI:
					// The Event sent on the channel is not the same as the EventAPI events so we need to type cast it
					eventsAPI, ok := event.Data.(slackevents.EventsAPIEvent)
					if !ok {
						log.Printf("Could not type cast the event to the EventsAPIEvent: %v\n", event)
						continue
					}
					// We need to send an Acknowledge to the slack server
					socket.Ack(*event.Request)
					// Now we have an Events API event, but this event type can in turn be many types, so we actually need another type switch

					//log.Println(eventsAPI) // commenting for event hanndling

					//------------------------------------
					// Now we have an Events API event, but this event type can in turn be many types, so we actually need another type switch
					err := HandleEventMessage(eventsAPI, client)
					if err != nil {
						// Replace with actual err handeling
						log.Fatal(err)
					}
				}
			}
		}
	}(ctx, client, socket)

	socket.Run()
}

// HandleEventMessage will take an event and handle it properly based on the type of event
func HandleEventMessage(event slackevents.EventsAPIEvent, client *slack.Client) error {
	switch event.Type {
	// First we check if this is an CallbackEvent
	case slackevents.CallbackEvent:

		innerEvent := event.InnerEvent
		// Yet Another Type switch on the actual Data to see if its an AppMentionEvent
		switch ev := innerEvent.Data.(type) {
		case *slackevents.AppMentionEvent:
			// The application has been mentioned since this Event is a Mention event
			err := HandleAppMentionEventToBot(ev, client)
			if err != nil {
				return err
			}
		}
	default:
		return errors.New("unsupported event type")
	}
	return nil
}

// HandleAppMentionEventToBot is used to take care of the AppMentionEvent when the bot is mentioned
func HandleAppMentionEventToBot(event *slackevents.AppMentionEvent, client *slack.Client) error {

	// Grab the user name based on the ID of the one who mentioned the bot
	user, err := client.GetUserInfo(event.User)
	if err != nil {
		return err
	}
	// Check if the user said Hello to the bot
	text := strings.ToLower(event.Text)

	// Create the attachment and assigned based on the message
	attachment := slack.Attachment{}

	if strings.Contains(text, "active_node") || strings.Contains(text, "!an") {
		// Greet the user

		ch := make(chan map[string]string)

		for region, url := range balancerSlice {
			go callBalancer(region, url, ch)
		}

		result := make(map[string]string)

		for i := 0; i < len(balancerSlice); i++ {
			outVal := <-ch
			for k, v := range outVal {
				result[k] = v
			}
		}

		var site1Result *HostResult
		var site2Result *HostResult
		var site3Result *HostResult
		for k, v := range result {
			if k == "site1" {
				switch v {
				case "blue":
					site1Result = new(HostResult)
					site1Result.Region = "site1"
					site1Result.ActiveNode = "Blue"
					site1Result.Build = callWebHost(hostsmap["ServerBlue1"])
				case "green":
					site1Result = new(HostResult)
					site1Result.Region = "site1"
					site1Result.ActiveNode = "Green"
					site1Result.Build = callWebHost(hostsmap["ServerGreen1"])
				}
			} else if k == "site2" {
				switch v {
				case "blue":
					site2Result = new(HostResult)
					site2Result.Region = "site2"
					site2Result.ActiveNode = "Blue"
					site2Result.Build = callWebHost(hostsmap["ServerBlue2"])
				case "green":
					site2Result = new(HostResult)
					site2Result.Region = "site2"
					site2Result.ActiveNode = "Green"
					site2Result.Build = callWebHost(hostsmap["ServerGreen2"])
				}
			} else if k == "site3" {
				switch v {
				case "blue":
					site3Result = new(HostResult)
					site3Result.Region = "site3"
					site3Result.ActiveNode = "Blue"
					site3Result.Build = callWebHost(hostsmap["ServerBlue3"])
				case "green":
					site3Result = new(HostResult)
					site3Result.Region = "site3"
					site3Result.ActiveNode = "Green"
					site3Result.Build = callWebHost(hostsmap["ServerGreen3"])
				}
			}

		}

		attachment.Text += fmt.Sprintf("Hello %s\n", user.RealName)
		attachment.Text += fmt.Sprintf("Site1 is %s Build <https://deploy.app.com/releases/%s|%s>\n", nodeColor(site1Result.ActiveNode), strings.Trim(strings.Trim(site1Result.Build, "]"), "["), site1Result.Build)
		attachment.Text += fmt.Sprintf("Site2 is %s Build <https://deploy.app.com/releases/%s|%s>\n", nodeColor(site2Result.ActiveNode), strings.Trim(strings.Trim(site2Result.Build, "]"), "["), site2Result.Build)
		attachment.Text += fmt.Sprintf("Site3 is %s Build <https://deploy.app.com/releases/%s|%s>\n", nodeColor(site3Result.ActiveNode), strings.Trim(strings.Trim(site3Result.Build, "]"), "["), site3Result.Build)

		attachment.Color = "#4af030"
	} else if strings.Contains(text, "help") {
		// Send a message to the user
		attachment.Text += fmt.Sprintf("Hello %s\n", user.RealName)
		attachment.Text += "Only one operation is avalable for now \n"
		attachment.Text += "active_node or !an - bot will display Site1\\Site2\\Site3 active node and deployed build \n"
		attachment.Color = "#4af030"
	} else {
		// Send a message to the user
		attachment.Text += fmt.Sprintf("Hello %s\n", user.RealName)
		attachment.Text += "Try to use 'help' with the bot name \n"
		attachment.Color = "#4af030"
	}
	// Send the message to the channel
	// The Channel is available in the event message
	_, _, err = client.PostMessage(event.Channel, slack.MsgOptionAttachments(attachment))
	if err != nil {
		return fmt.Errorf("failed to post message: %w", err)
	}
	return nil
}

func callBalancer(region string, url string, chBalancerResult chan map[string]string) {
	innerBalancerResult := make(map[string]string)

	var Balancerresult Balancer
	json.Unmarshal([]byte(`{"farms":[{"active":null,"enabled":"True","name":"site1.domain.com-green","nodes":null}],"hosts":null,"nodes":null,"response":null,"status":"success"}`), &Balancerresult)

	str := Balancerresult.Farms[0].Name
	switch {
	case strings.Contains(str, "blue"):
		innerBalancerResult[region] = "blue"

	case strings.Contains(str, "green"):
		innerBalancerResult[region] = "green"
	}

	chBalancerResult <- innerBalancerResult
}

func callWebHost(url string) string {
	resBody := `SUCCESS: ServerName
	Build: [20210314.1.2730-new-cool-ui-feature]<br>
	Server IP: 172.0.0.1`

	buildVersion := matchRegEx(string(resBody))
	return buildVersion
}

func matchRegEx(input string) string {

	result := ""
	re := regexp.MustCompile(`Build:\s*(.*?)\s*<br>`)

	matches := re.FindAllStringSubmatch(input, -1)
	for _, v := range matches {
		result = v[1]
	}

	return result
}

func nodeColor(nodeColoer string) string {

	nodeColoer = strings.ToLower(nodeColoer)
	if nodeColoer == "blue" {
		return ":large_blue_circle:"
	} else if nodeColoer == "green" {
		return ":green_circle:"
	} else {
		return ":goose:"
	}
}
