/// <reference path="../.astro/types.d.ts" />
/// <reference types="astro/client" />

interface ImportMetaEnv {
  readonly VIDEO_SERVER_URL: string;
  readonly VIDEO_SERVER_SECRET: string;
  readonly DATABASE_URL: string;
  readonly R2_ACCOUNT_ID: string;
  readonly R2_ACCESS_KEY_ID: string;
  readonly R2_SECRET_ACCESS_KEY: string;
  readonly R2_BUCKET_NAME: string;
  readonly R2_ENDPOINT: string;
}

interface ImportMeta {
  readonly env: ImportMetaEnv;
}
