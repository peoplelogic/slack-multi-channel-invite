# slack-multi-channel-invite
Thanks to all those that came before to help solve this for Peoplelogic!

Old Readme:
Have you ever googled `"slack invite user to multiple channels"`?  Yeah, me too.  I do this every time a new engineer joins my team, and I inevitably end up inviting said engineer to each Slack channel manually.  I got tired of this, so I rolled up my sleeves and whipped up this script.

I assume Slack will eventually add this ability.  Until then, hopefully you can save some time by using this.

Enjoy!

## Instructions
1. [Create](https://api.slack.com/apps) a Slack App for your workspace.
2. Add the following permission scopes to a user token (bot tokens aren't allowed `channels:write`):
    - `users:read`
    - `users:read.email`
    - `channels:read`
    - `channels:write`
    - `groups:read` (only if inviting to private channels)
    - `groups:write` (only if inviting to private channels)
3. Install app to your workspace which will generate a new User OAuth token
4. [Install Go to your local machine](https://go.dev/doc/install)
5. Download script:
    - Download release from [https://github.com/peoplelogic/slack-multi-channel-invite/releases](https://github.com/peoplelogic/slack-multi-channel-invite/releases/tag/release-0.1.0)
    - Unzip release to local directory
    - Change to the directory of the script
6. Run script:

`go run main.go -api_token=<user-oauth-token> -emails=steph@warriors.com,klay@warriors.com -channels=dubnation,splashbrothers,thetown -private=<true|false> -list=<true|false>`

The users with emails `steph@warriors.com` and `klay@warriors.com` should be invited to channels `dubnation`, `splashbrothers`, and `thetown`!

_* Set `private` flag to `true` if you want to invite users to private channels.  As noted above, this will require the additional permission scopes of `groups:read` and `groups:write`_

_* The behaviour of the `list` flag set to `true` depends on whether the `emails` is listing a set of emails or not. When `emails` is empty, it simply lists the available channels, including the private ones if `private` is also set to true. When `emails` is not empty instead it will list the channels that these users are part of, always including the private ones. This will also require the additional permission scopes of `groups:read` and `groups:write`._

#### Want to remove users from channels?
Simply set the optional `action` flag to `remove` (`add` is the default):

`go run main.go -api_token=<user-oauth-token> -action=remove -emails=kd@warriors.com -channels=dubnation,warriors -private=<true|false>`

## Using it with Github Actions

You can also automate this using Github Actions and [Github Secrets](https://docs.github.com/en/actions/security-guides/encrypted-secrets) for your API key:
```
name: "ManageSlackUsers"
on:
  push:
    branches:
      - master
      - main

jobs:
    managing_users:
      runs-on: ubuntu-latest
      steps:
        - uses: actions/checkout@v3
        - name: Setup go
          uses: actions/setup-go@v3
          with:
            go-version: '1.18'
        - run: go mod tidy
        - run: go run main.go -private -api_token="${{ secrets.SLACK_API_KEY }}" -action "$(cat list.txt | tail -n 2 | head -n 1 | cut -d ':' -f 1)" -emails "$(cat list.txt | tail -n 2 | head -n 1 | cut -d ':' -f 2)" -channels "$(cat list.txt | tail -n 1)"
```
which would read from the local file `list.txt` the last two lines and take actions accordingly. E.g. your file might look like:
```
Adding people to slack channel 
add:someemail@example.com,otherone@test.com
user-stories,lobby,secretchannel

Now listing users
list:
user-stories
```
This would only execute the last two lines, and so it would list users in the channel `#user-stories`.
This means you can now manage your Slack users using a Github action by simply editing the `list.txt` file in Github directly.

Possible actions are: `add:`, `remove:` and `list:`.

## Implementation
Initially, I figured this script would be a simple loop that invoked some API to invite users to a channel.  It turns out this API endpoint ([`conversations.invite`](https://api.slack.com/methods/conversations.invite)) expects the user ID (instead of username) and channel ID (instead of channel name).  Problem is, it's not very straightforward to get user and channel IDs. There isn't a way to lookup a user by username (only by email).  And there's no way to look up a single channel, unless you have the channel ID already (chicken and egg).

For these reasons, I wrote the script like so:
1. [Look up](https://api.slack.com/methods/users.lookupByEmail) Slack user IDs for all given emails.
2. [Query](https://api.slack.com/methods/conversations.list) all public (or private) channels in the workspace and create a name -> ID mapping.
3. For each of the given channels, [invite](https://api.slack.com/methods/conversations.invite) the users to the channel using the user IDs and channel ID from steps 1 & 2.
