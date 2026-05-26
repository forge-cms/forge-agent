package main

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	_ "time/tzdata" // embed timezone database for Alpine/scratch containers

	"smeldr.dev/agent"
)

const electricitySystemPrompt = `You are a daily electricity price advisor for a Danish household.

Your job:

1. Fetch 48 hours of DK2 day-ahead prices using http_get:
   https://api.energidataservice.dk/dataset/DayAheadPrices?limit=192&filter={"PriceArea":"DK2"}&sort=TimeUTC%20desc

2. Parse the JSON response. Each record has TimeUTC (UTC timestamp) and DayAheadPriceDKK
   (price in DKK per MWh — divide by 1000 to get kr/kWh). Data is in 15-minute intervals;
   a 2-hour block is 8 consecutive records. Note: DayAheadPriceDKK is derived from the EUR
   price using Nationalbanken's exchange rate and may differ slightly from other price trackers.

3. Group records by calendar date (UTC). The two most recent dates in the response
   are today and tomorrow (Denmark is CEST = UTC+2 in summer, CET = UTC+1 in winter).
   The more recent date = tomorrow. The earlier date = today.

4. For each date group, find the cheapest consecutive 2-hour block (8 consecutive 15-minute records).

5. Post a concise recommendation in Danish (max 200 characters) via http_post:
   url: DISCORD_WEBHOOK_URL_PLACEHOLDER
   content_type: application/json
   body: {"content": "I dag: billigst 13-15 (0.42 kr/kWh). I morgen: billigst 03-05 (0.38 kr/kWh)."}

Keep the message short and actionable — it is a push notification.`

func main() {
	discordWebhookURL := requireEnv("DISCORD_WEBHOOK_URL")

	prompt := strings.ReplaceAll(electricitySystemPrompt, "DISCORD_WEBHOOK_URL_PLACEHOLDER", discordWebhookURL)

	jobs := []agent.Job{
		{
			Schedule: "45 13 * * *",
			Timezone: "Europe/Copenhagen",
			Task:     "Find the cheapest 2-hour EV charging window for today and tomorrow, then post the recommendation to Discord.",
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

	runNow := make(chan os.Signal, 1)
	notifyUSR1(runNow)

	for {
		select {
		case <-quit:
			return
		case <-runNow:
			go func() {
				result, err := agent.New(jobs[0].Config).Run(context.Background(), jobs[0].Task)
				if err != nil {
					fmt.Fprintln(os.Stderr, "manual run error:", err)
					return
				}
				fmt.Fprintln(os.Stderr, "manual run done:", result)
			}()
		}
	}
}

// requireEnv returns the value of the named environment variable.
// Prints usage and exits non-zero if the variable is empty.
func requireEnv(key string) string {
	v := os.Getenv(key)
	if v == "" {
		fmt.Fprintf(os.Stderr, "error: required environment variable %s is not set\n", key)
		fmt.Fprintln(os.Stderr, "")
		fmt.Fprintln(os.Stderr, "Required environment variables:")
		fmt.Fprintln(os.Stderr, "  ANTHROPIC_API_KEY     Anthropic API key (read by the SDK automatically)")
		fmt.Fprintln(os.Stderr, "  DISCORD_WEBHOOK_URL   Discord webhook URL for push notifications")
		os.Exit(1)
	}
	return v
}
