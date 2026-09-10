"use strict";

const PLATFORMS = Object.freeze({
  "darwin arm64": "@etherscan/cli-darwin-arm64",
  "darwin x64": "@etherscan/cli-darwin-x64",
  "linux arm64": "@etherscan/cli-linux-arm64",
  "linux x64": "@etherscan/cli-linux-x64",
  "win32 arm64": "@etherscan/cli-win32-arm64",
  "win32 x64": "@etherscan/cli-win32-x64",
});

function platformPackage(platform, arch) {
  return PLATFORMS[`${platform} ${arch}`] || null;
}

module.exports = { PLATFORMS, platformPackage };
