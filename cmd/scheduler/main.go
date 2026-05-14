package main

import (
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	_ "time/tzdata" // embed timezone database for Alpine/scratch containers

	agent "forge-cms.dev/forge-agent"
)

const electricitySystemPrompt = `You are a daily electricity price advisor for a Danish household.

Your job:

1. Fetch 48 hours of DK2 spot prices using http_get:
   https://api.energidataservice.dk/dataset/Elspotprices?limit=48&filter={"PriceArea":"DK2"}&sort=HourUTC%20asc

2. Parse the JSON response. Each record has HourUTC and SpotPriceDKK fields.
   SpotPriceDKK is the price in DKK per MWh — divide by 1000 to get kr/kWh.

3. Split the data into two 24-hour windows:
   - Window A: the next 24 hours (today)
   - Window B: the following 24 hours (tomorrow)

4. For each window, find the cheapest consecutive 2-hour block.

5. Post a concise recommendation in Danish (max 200 characters) via http_post:
   url: https://ntfy.sh/NTFY_TOPIC_PLACEHOLDER
   content_type: text/plain
   body: e.g. "I dag: billigst 02-04 (0.42 kr/kWh). I morgen: billigst 03-05 (0.38 kr/kWh)."

Keep the message short and actionable — it is a push notification.`

func main() {
	ntfyTopic := requireEnv("NTFY_TOPIC")

	prompt := strings.ReplaceAll(electricitySystemPrompt, "NTFY_TOPIC_PLACEHOLDER", ntfyTopic)

	jobs := []agent.Job{
		{
			Schedule: "0 6 * * *",
			Timezone: "Europe/Copenhagen",
			Task:     "Find the cheapest 2-hour EV charging window for today and tomorrow, then post the recommendation to ntfy.sh.",
			Config: agent.Config{
				SystemPrompt: prompt,
			},
		},
	}

	s, err := agent.NewScheduler(jobs)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}

	s.Start()
	defer s.Stop()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit
}

// requireEnv returns the value of the named environment variable.
// Prints usage and exits non-zero if the variable is empty.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "error: required environment variable %s is not set\n", key)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Required environment variables:")
		fmt.Fprintln(os.Stderr, "  ANTHROPIC_API_KEY   Anthropic API key (read by the SDK automatically)")
		fmt.Fprintln(os.Stderr, "  NTFY_TOPIC          ntfy.sh topic name for push notifications")
		os.Exit(1)
	}
	return v
}
