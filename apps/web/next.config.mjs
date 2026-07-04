/** @type {import('next').NextConfig} */
const nextConfig = {
  reactStrictMode: true,
  async rewrites() {
    return [
      {
        source: '/api/:path*',
        destination: 'http://localhost:8080/api/:path*',
      },
      {
        source: '/api-health',
        destination: 'http://localhost:8080/health',
      },
      {
        source: '/ai-health',
        destination: 'http://localhost:8000/health',
      },
    ];
  },
};

export default nextConfig;
