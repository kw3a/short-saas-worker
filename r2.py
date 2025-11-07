import os
import boto3
from dotenv import load_dotenv

def _r2_client():
    ACCESS_KEY = os.environ["R2_ACCESS_KEY_ID"]
    SECRET_ACCESS_KEY = os.environ["R2_SECRET_ACCESS_KEY"]
    R2_ACCOUNT_ID = os.environ["R2_ACCOUNT_ID"]
    return boto3.client(
        service_name ="s3",
        endpoint_url = f"https://{R2_ACCOUNT_ID}.r2.cloudflarestorage.com",
        aws_access_key_id = ACCESS_KEY,
        aws_secret_access_key = SECRET_ACCESS_KEY,
        region_name="auto",
    )

def upload_to_S3(file_path: str, job_id: str):
    R2_BUCKET_NAME = os.environ.get("R2_BUCKET_NAME", "shorts")
    s3 = _r2_client()
    # Key in R2 bucket
    file_key = f"{job_id}.mp4"

    # Upload to R2 from local file path
    s3.upload_file(
        Filename=file_path,
        Bucket=R2_BUCKET_NAME,
        Key=file_key,
        ExtraArgs={"ContentType": "video/mp4"},
    )

    # Generate presigned URL valid for a limited time

def upload_thumbnail_to_S3(file_path: str, job_id: str):
    R2_BUCKET_NAME = os.environ.get("R2_BUCKET_NAME", "shorts")
    s3 = _r2_client()
    file_key = f"{job_id}.jpg"
    s3.upload_file(
        Filename=file_path,
        Bucket=R2_BUCKET_NAME,
        Key=file_key,
        ExtraArgs={"ContentType": "image/jpeg"},
    )
    
    #signed_url = s3.generate_presigned_url(
    #    ClientMethod='get_object',
    #    Params={'Bucket': R2_BUCKET_NAME, 'Key': file_key},
    #    ExpiresIn=expires_in,
    #)
    #return signed_url

if __name__ == "__main__":
    load_dotenv()
    upload_to_S3("test2.mp4", "12345678")