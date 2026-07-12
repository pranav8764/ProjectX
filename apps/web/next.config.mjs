/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    const apiURL = process.env.NEXT_API_URL || 'http://localhost:8080';
    const aiURL = process.env.NEXT_AI_URL || 'http://localhost:8000';
    return [
      {
        // Proxy everything under /api EXCEPT /api/auth/* — those are handled locally
        // by the BetterAuth route handler (app/api/auth/[...all]). The negative
        // lookahead keeps auth requests from being forwarded to the Go gateway.
        source: '/api/:path((?!auth/).*)',
        destination: `${apiURL}/api/:path`,
      },
      {
        source: '/api-health',
        destination: `${apiURL}/health`,
      },
      {
        source: '/ai-health',
        destination: `${aiURL}/health`,
      },
    ];
  },
};

export default nextConfig;
