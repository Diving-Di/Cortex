const path = require('path')
const HtmlWebpackPlugin = require('html-webpack-plugin')

module.exports = (env, argv) => {
  const isProd = argv.mode === 'production'
  const backendUrl = process.env.BACKEND_URL || 'http://127.0.0.1:8000'

  return {
    entry: './src/main.tsx',
    output: {
      path: path.resolve(__dirname, 'dist'),
      filename: isProd ? '[name].[contenthash].js' : '[name].js',
      publicPath: '/',
      clean: true,
    },
    resolve: {
      extensions: ['.tsx', '.ts', '.jsx', '.js'],
    },
    module: {
      rules: [
        {
          test: /\.[jt]sx?$/,
          exclude: /node_modules/,
          use: {
            loader: 'ts-loader',
            options: { transpileOnly: true },
          },
        },
        {
          test: /\.css$/,
          use: ['style-loader', 'css-loader'],
        },
      ],
    },
    plugins: [
      new HtmlWebpackPlugin({
        template: './index.html',
      }),
    ],
    devServer: {
      port: 5173,
      host: '0.0.0.0',
      historyApiFallback: true,
      proxy: [
        {
          context: ['/api', '/media'],
          target: backendUrl,
          changeOrigin: true,
        },
      ],
    },
    devtool: isProd ? false : 'eval-source-map',
  }
}
