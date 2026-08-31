module.exports = {
    publicPath: '/ui',
    outputDir: './ui',
    productionSourceMap: false,
    configureWebpack: {
        performance: {
            maxAssetSize: 1000000,
            maxEntrypointSize: 1500000,
        },
    },
}
