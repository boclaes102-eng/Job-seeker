"""Start Job Seeker:  python run.py   then open http://127.0.0.1:8000"""
import uvicorn

from jobseeker.config import CONFIG

if __name__ == "__main__":
    print(f"Job Seeker running at http://{CONFIG.host}:{CONFIG.port}")
    uvicorn.run("jobseeker.api:app", host=CONFIG.host, port=CONFIG.port, reload=False)
