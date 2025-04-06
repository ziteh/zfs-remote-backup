from datetime import datetime

with open("/app/cron_test.log", "a") as f:
    f.write(f"[{datetime.now()}] Cron job executed\n")
