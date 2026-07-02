import { S3Client, GetObjectCommand, HeadObjectCommand } from "@aws-sdk/client-s3";
import { getSignedUrl } from "@aws-sdk/s3-request-presigner";

// Mirrors ../../app/src/lib/r2.ts, pointed at whatever bucket go-video itself
// uploads to (internal/storage/r2.go): real R2 in prod, or a local MinIO
// container in dev when R2_ENDPOINT is set (see ../../go-video/README.md).
const endpoint = import.meta.env.R2_ENDPOINT || `https://${import.meta.env.R2_ACCOUNT_ID}.r2.cloudflarestorage.com`;

const r2 = new S3Client({
  region: "auto",
  endpoint,
  forcePathStyle: true,
  credentials: {
    accessKeyId: import.meta.env.R2_ACCESS_KEY_ID ?? "",
    secretAccessKey: import.meta.env.R2_SECRET_ACCESS_KEY ?? "",
  },
});

const bucket = import.meta.env.R2_BUCKET_NAME ?? "shorts";

async function signedUrlIfExists(key: string, contentType: string): Promise<string | null> {
  try {
    await r2.send(new HeadObjectCommand({ Bucket: bucket, Key: key }));
  } catch (err: any) {
    if (err?.name === "NotFound" || err?.$metadata?.httpStatusCode === 404) return null;
    throw err;
  }
  const command = new GetObjectCommand({ Bucket: bucket, Key: key, ResponseContentType: contentType });
  return getSignedUrl(r2, command, { expiresIn: 3600 });
}

export function getVideoUrlIfReady(id: string) {
  return signedUrlIfExists(`${id}.mp4`, "video/mp4");
}

export function getThumbUrlIfReady(id: string) {
  return signedUrlIfExists(`${id}.jpg`, "image/jpeg");
}
