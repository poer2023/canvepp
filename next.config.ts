import type { NextConfig } from 'next';
const config: NextConfig = { distDir: process.env.CANVEPP_ACCEPTANCE_BUILD === '1' ? '.next-acceptance' : '.next', serverExternalPackages: ['better-sqlite3', 'sharp'], devIndicators: false, outputFileTracingExcludes: { '/*': ['./.data/**/*', './artifacts/**/*', './.next-acceptance/**/*'] }, turbopack: { root: process.cwd() } };
export default config;
