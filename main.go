package main

import (
	"bytes"
	"encoding/json"
	"flag"
	"fmt"
	"io/ioutil"
	"net/http"
	"os"
	"sort"
	"strings"

	"golang.org/x/exp/maps"
	"golang.org/x/exp/slices"
)

const (
	conversationsInviteURL   = "https://slack.com/api/conversations.invite"
	conversationsKickURL     = "https://slack.com/api/conversations.kick"
	conversationsListURL     = "https://slack.com/api/conversations.list"
	conversationsUserListURL = "https://slack.com/api/conversations.members"
	usersLookupByEmailURL    = "https://slack.com/api/users.lookupByEmail"
	usersLookupByIdURL       = "https://slack.com/api/users.info"

	actionAdd    = "add"
	actionRemove = "remove"
	actionList   = "list"
)

type (
	conversationsListResponse struct {
		Ok               bool             `json:"ok"`
		Channels         []channel        `json:"channels"`
		ResponseMetadata responseMetadata `json:"response_metadata"`
		Error            string           `json:error`
	}

	conversationsMembersResponse struct {
		Ok               bool             `json:"ok"`
		Members          []string         `json:"members"`
		ResponseMetadata responseMetadata `json:"response_metadata"`
		Error            string           `json:error`
	}

	channel struct {
		ID   string `json:"id"`
		Name string `json:"name"`
	}

	responseMetadata struct {
		NextCursor string `json:"next_cursor"`
	}

	conversationsInviteRequest struct {
		ChannelID string `json:"channel"`
		UserIDs   string `json:"users"`
	}

	conversationsInviteResponse struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error"`
	}

	conversationsKickRequest struct {
		ChannelID string `json:"channel"`
		UserID    string `json:"user"`
	}

	conversationsKickResponse struct {
		Ok    bool   `json:"ok"`
		Error string `json:"error"`
	}

	usersLookupResponse struct {
		Ok    bool   `json:"ok"`
		User  user   `json:"user"`
		Error string `json:"error"`
	}

	user struct {
		ID       string `json:"id"`
		Name     string `json:"name"`
		RealName string `json:"real_name"`
	}
)

func getUsersIdsFrom(apiToken, emails string) []string {
	userIDs := []string{}
	var err error
	for _, email := range strings.Split(emails, ",") {
		var userID string
		if strings.Contains(email, "@") {
			userID, err = getUserID(apiToken, email)
			if err != nil {
				fmt.Printf("Error while looking up user with email %s: %s\n", email, err)
				continue
			}
			fmt.Printf("Valid user (ID: %s) found for '%s'\n", userID, email)
		} else {
			userName, realName, err := getUserName(apiToken, email)
			if err != nil {
				fmt.Println("Invalid user provided:", email, err)
				continue
			}
			userID = email
			fmt.Printf("Valid user (ID: %s) provided for %s (%s)\n", userID, realName, userName)
		}
		userIDs = append(userIDs, userID)
	}
	return userIDs
}

// This script invites the given users to the given channels on Slack.
// Due to the oddness of the Slack API, this is accomplished via these steps:
// 1) Look up Slack user IDs by email
// 2) Query all public (private if 'private' flag is set to true) channels in the workspace and create a name -> ID mapping
// 3) For each of the given channels, invite the users (user IDs) to the channel (channel ID)
func main() {
	var apiToken string
	var action string
	var emails string
	var channelsArg string
	var private bool
	var listChannels bool
	var debug bool

	// parse flags
	flag.StringVar(&apiToken, "api_token", "", "Slack OAuth Access Token")
	flag.StringVar(&action, "action", "add", "'add' to invite users, 'remove' to remove users")
	flag.StringVar(&emails, "emails", "", "Comma separated list of Slack user emails to invite, or user IDs")
	flag.StringVar(&channelsArg, "channels", "", "Comma separated list of channels to invite users to, or to list users for")
	flag.BoolVar(&private, "private", false, "Boolean flag to enable private channel invitations (requires OAuth scopes 'groups:read' and 'groups:write')")
	flag.BoolVar(&listChannels, "list", false, "Boolean flag to list channels, or list users in given channels if used with -channels")
	flag.BoolVar(&debug, "debug", false, "Enables debug logging when set to true")
	flag.Parse()

	if apiToken == "" {
		flag.Usage()
		os.Exit(1)
	}

	// get all channels
	channelNameToIDMap, err := getChannels(apiToken, private, debug)
	if err != nil {
		panic(err)
	}

	if action == actionList {
		listChannels = true
	}

	if listChannels {
		if channelsArg == "" && emails == "" {
			fmt.Println("List of found channels (use -private to include private channels):")
			keys := maps.Keys(channelNameToIDMap)
			sort.Strings(keys)
			max := 0
			for _, k := range keys {
				if len(k) > max {
					max = len(k)
				}
			}
			sb := &strings.Builder{}
			for _, k := range keys {
				fmt.Printf("\t • %-*s  --> %s\n", max+3, k, channelNameToIDMap[k])
				fmt.Fprintf(sb, "%s,", k)
			}
			fmt.Println(sb.String())
			return
		} else if emails == "" {
			channels := strings.Split(channelsArg, ",")
			for _, channel := range channels {
				channelID := channelNameToIDMap[channel]
				if channelID == "" {
					fmt.Printf("Channel '%s' not found -- skipping\n", channel)
					continue
				}
				fmt.Println("Listing users for channel", channel)
				users, err := getUsersById(apiToken, channelID, debug)
				if err != nil {
					fmt.Println("Error while listing users for channel", channel, err)
					continue
				}
				max := 0
				for _, v := range users {
					if len(v) > max {
						max = len(v)
					}
				}
				sb := &strings.Builder{}
				for _, v := range users {
					name, realname, err := getUserName(apiToken, v)
					if err != nil {
						fmt.Println("Error while getting user name for", v)
						continue
					}
					fmt.Printf("\t\t • %-*s --> %s (%s)\n", max+3, v, realname, name)
					fmt.Fprintf(sb, "%s,", v)
				}
				fmt.Println("\tFull list of users:\n", sb.String(), "\n for channel", channel)
			}
			return
		} else {
			userids := getUsersIdsFrom(apiToken, emails)
			fmt.Println("Listing channels the provided users are part of.")
			for _, id := range userids {
				fmt.Println("User", id, "is part of the following channels:")
				channels, err := getAllChannelsForUser(apiToken, id, debug)
				if err != nil {
					os.Exit(1)
				}
				for _, v := range channels {
					fmt.Println("\t", v)
				}
			}
		}
		fmt.Println("--list does not do any further action")
		return
	}

	if emails == "" || channelsArg == "" || (action != actionAdd && action != actionRemove) {
		if listChannels {
			fmt.Println("Listing channels done, please use proper flags to perform actions.")
		}
		flag.Usage()
		os.Exit(1)
	}

	// lookup users by email
	fmt.Printf("\nLooking up users ...\n")
	userIDs := getUsersIdsFrom(apiToken, emails)
	if (action == actionAdd || action == actionRemove) && len(userIDs) == 0 {
		fmt.Println("\nNo users found - aborting")
		os.Exit(1)
	}

	if debug {
		fmt.Printf("DEBUG: Total # of channels retrieved: %d\n", len(channelNameToIDMap))
	}

	// invite/remove users to each channel
	if action == actionAdd {
		fmt.Printf("\nInviting users to channels ...\n")
	} else if action == actionRemove {
		fmt.Printf("\nRemoving users from channels ...\n")
	} else {
		fmt.Println("ERROR: invalid action / flag combination")
		os.Exit(1)
	}

	channels := strings.Split(channelsArg, ",")

	for _, channel := range channels {
		channelID := channelNameToIDMap[channel]
		if channelID == "" {
			fmt.Printf("Channel '%s' not found -- skipping\n", channel)
			continue
		}

		if action == actionAdd {
			err := inviteUsersToChannel(apiToken, userIDs, channelID, channel)
			if err != nil {
				fmt.Printf("Error while inviting users to %s (%s): %s\n", channel, channelID, err)
				continue
			}
		} else {
			err := removeUsersFromChannel(apiToken, userIDs, channelID, channel, debug)
			if err != nil {
				fmt.Printf("Error while removing users from %s (%s): %s\n", channel, channelID, err)
				continue
			}
		}

		if action == actionAdd {
			fmt.Printf("Users invited to '%s'\n", channel)
		} else {
			fmt.Printf("Users removed from '%s'\n", channel)
		}
	}

	fmt.Println("\nAll done! You're welcome =)")
}

func getUserName(apiToken, userID string) (string, string, error) {
	httpClient := &http.Client{}

	// lookup user by ID
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(usersLookupByIdURL+"?user=%s", userID), nil)
	if err != nil {
		return "", "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return "", "", err
		}
		return "", "", fmt.Errorf("Non-200 status code (%d)", resp.StatusCode)
	}

	var data usersLookupResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", "", err
	}

	if !data.Ok {
		fmt.Printf("usersLookupResponse: %+v\n", data)
		return "", "", fmt.Errorf("Non-ok response while looking up user by email")
	}

	// return user Name
	return data.User.Name, data.User.RealName, nil
}

func getUserID(apiToken, userEmail string) (string, error) {
	httpClient := &http.Client{}

	// lookup user by email
	req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(usersLookupByEmailURL+"?email=%s", userEmail), nil)
	if err != nil {
		return "", err
	}

	req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return "", err
		}
		return "", fmt.Errorf("Non-200 status code (%d)", resp.StatusCode)
	}

	var data usersLookupResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return "", err
	}

	if !data.Ok {
		fmt.Printf("usersLookupByEmailResponse: %+v\n", data)
		return "", fmt.Errorf("Non-ok response while looking up user by email")
	}

	// return user ID
	return data.User.ID, nil
}

func getAllChannelsForUser(apiToken, userID string, debug bool) ([]string, error) {
	memberof := sort.StringSlice{}
	channels, err := getChannels(apiToken, true, debug)
	if err != nil {
		return nil, err
	}
	for cname, cid := range channels {
		users, err := getUsersById(apiToken, cid, debug)
		if err != nil {
			return nil, err
		}
		if slices.Contains(users, userID) {
			memberof = append(memberof, cname)
		}
	}
	memberof.Sort()
	return memberof, nil
}

func getUsersById(apiToken, channelID string, debug bool) ([]string, error) {
	members := make([]string, 0, 50)
	httpClient := &http.Client{}
	var nextCursor string
	for {
		// query list of channels
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(conversationsUserListURL+"?cursor=%s&limit=200&channel=%s", nextCursor, channelID), nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err := printErrorResponseBody(resp)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("Non-200 status code (%d)", resp.StatusCode)
		}

		var data conversationsMembersResponse
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			return nil, err
		}

		if !data.Ok {
			fmt.Printf("conversationsMembersResponse: %+v", data)
			return nil, fmt.Errorf("Non-ok response while querying list of users for channel '%s'", channelID)
		}

		if debug {
			fmt.Printf("DEBUG: # of users returned in page: %d\n", len(data.Members))
		}

		// map of channel names to IDs
		for _, user := range data.Members {
			members = append(members, user)
		}

		// paginate if necessary
		nextCursor = data.ResponseMetadata.NextCursor
		if nextCursor == "" {
			break
		}
	}

	return members, nil
}
func getChannels(apiToken string, private bool, debug bool) (map[string]string, error) {

	channelType := "public_channel"
	if private {
		channelType = "private_channel,public_channel"
	}

	nameToID := make(map[string]string)

	httpClient := &http.Client{}
	var nextCursor string
	for {
		// query list of channels
		req, err := http.NewRequest(http.MethodGet, fmt.Sprintf(conversationsListURL+"?cursor=%s&exclude_archived=true&limit=200&types=%s", nextCursor, channelType), nil)
		if err != nil {
			return nil, err
		}

		req.Header.Add("Content-Type", "application/x-www-form-urlencoded")
		req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

		resp, err := httpClient.Do(req)
		if err != nil {
			return nil, err
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			err := printErrorResponseBody(resp)
			if err != nil {
				return nil, err
			}
			return nil, fmt.Errorf("Non-200 status code (%d)", resp.StatusCode)
		}

		var data conversationsListResponse
		err = json.NewDecoder(resp.Body).Decode(&data)
		if err != nil {
			return nil, err
		}

		if !data.Ok {
			fmt.Printf("conversationsListResponse: %+v", data)
			return nil, fmt.Errorf("Non-ok response while querying list of channels")
		}

		if debug {
			fmt.Printf("DEBUG: # of channels returned in page: %d\n", len(data.Channels))
		}

		// map of channel names to IDs
		for _, channel := range data.Channels {
			nameToID[channel.Name] = channel.ID
		}

		// paginate if necessary
		nextCursor = data.ResponseMetadata.NextCursor
		if nextCursor == "" {
			break
		}
	}

	return nameToID, nil
}

func inviteUsersToChannel(apiToken string, userIDs []string, channelID, channelName string) error {
	httpClient := &http.Client{}

	reqBody, err := json.Marshal(conversationsInviteRequest{
		ChannelID: channelID,
		UserIDs:   strings.Join(userIDs, ","),
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, conversationsInviteURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return err
		}
		return fmt.Errorf("Non-200 status code: (%d)", resp.StatusCode)
	}

	var data conversationsInviteResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return err
	}

	if !data.Ok {
		if data.Error == "already_in_channel" {
			fmt.Println("User already in channel:", channelName)
			return nil
		}
		fmt.Printf("conversationsInviteResponse: %+v\n", data)
		return fmt.Errorf("Non-ok response while inviting user to channel")
	}

	return nil
}

func removeUsersFromChannel(apiToken string, userIDs []string, channelID, channelName string, debug bool) error {
	// API only supports removing users one at a time ...
	fmt.Println("Removing users from channel:", channelName)
	for _, userID := range userIDs {
		err := removeUserFromChannel(apiToken, userID, channelID)
		if err != nil {
			if debug {
				fmt.Printf("DEBUG: Error while removing user %s from channel %s: %s\n", userID, channelID, err)
			}
			return err
		}
	}
	return nil
}

func removeUserFromChannel(apiToken string, userID string, channelID string) error {
	httpClient := &http.Client{}

	reqBody, err := json.Marshal(conversationsKickRequest{
		ChannelID: channelID,
		UserID:    userID,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequest(http.MethodPost, conversationsKickURL, bytes.NewReader(reqBody))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", apiToken))

	resp, err := httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		err := printErrorResponseBody(resp)
		if err != nil {
			return err
		}
		return fmt.Errorf("Non-200 status code: (%d)", resp.StatusCode)
	}

	var data conversationsKickResponse
	err = json.NewDecoder(resp.Body).Decode(&data)
	if err != nil {
		return err
	}

	if !data.Ok {
		fmt.Printf("conversationsKickResponse: %+v\n", data)
		return fmt.Errorf("Non-ok response while removing user from channel")
	}

	return nil
}

func printErrorResponseBody(resp *http.Response) error {
	bodyBytes, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return err
	}
	fmt.Println(string(bodyBytes))

	return nil
}
