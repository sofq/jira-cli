#!/usr/bin/env node
"use strict";

const { execSync } = require("child_process");
const fs = require("fs");
const path = require("path");
const https = require("https");
const { createWriteStream, mkdirSync } = require("fs");
const { pipeline } = require("stream/promises");

const REPO = "sofq/jira-cli";
const BIN_DIR = path.join(__dirname, "bin");

const PLATFORM_MAP = {
  darwin: "darwin",
  linux: "linux",
  win32: "windows",
};

const ARCH_MAP = {
  x64: "amd64",
  arm64: "arm64",
};

async function getVersion() {
  const pkg = require("./package.json");
  return pkg.version;
}

function getPlatformArch() {
  const platform = PLATFORM_MAP[process.platform];
  const arch = ARCH_MAP[process.arch];
  if (!platform || !arch) {
    console.error(
      `Unsupported platform/arch: ${process.platform}/${process.arch}`
    );
    process.exit(1);
  }
  return { platform, arch };
}

function getDownloadUrl(version, platform, arch) {
  const ext = platform === "windows" ? "zip" : "tar.gz";
  const name = `jira-cli_${version}_${platform}_${arch}.${ext}`;
  return `https://github.com/${REPO}/releases/download/v${version}/${name}`;
}

function follow(url) {
  return new Promise((resolve, reject) => {
    https
      .get(url, { headers: { "User-Agent": "jr-npm-installer" } }, (res) => {
        if (res.statusCode >= 300 && res.statusCode < 400 && res.headers.location) {
          return follow(res.headers.location).then(resolve, reject);
        }
        if (res.statusCode !== 200) {
          return reject(new Error(`HTTP ${res.statusCode} for ${url}`));
        }
        resolve(res);
      })
      .on("error", reject);
  });
}

async function install() {
  const version = await getVersion();
  const { platform, arch } = getPlatformArch();
  const url = getDownloadUrl(version, platform, arch);
  const binName = platform === "windows" ? "jr.exe" : "jr";
  const binPath = path.join(BIN_DIR, binName);

  if (fs.existsSync(binPath)) {
    return;
  }

  mkdirSync(BIN_DIR, { recursive: true });

  console.log(`Downloading jr v${version} for ${platform}/${arch}...`);
  const res = await follow(url);

  if (platform === "windows") {
    // Download zip, extract with unzip
    const tmpZip = path.join(BIN_DIR, "jr.zip");
    const ws = createWriteStream(tmpZip);
    await pipeline(res, ws);
    execSync(`unzip -o "${tmpZip}" jr.exe -d "${BIN_DIR}"`, { stdio: "pipe" });
    fs.unlinkSync(tmpZip);
  } else {
    // Stream tar.gz and extract
    const tmpTar = path.join(BIN_DIR, "jr.tar.gz");
    const ws = createWriteStream(tmpTar);
    await pipeline(res, ws);
    execSync(`tar xzf "${tmpTar}" -C "${BIN_DIR}" jr`, { stdio: "pipe" });
    fs.unlinkSync(tmpTar);
  }

  fs.chmodSync(binPath, 0o755);
  console.log(`Installed jr to ${binPath}`);
}

install().catch((err) => {
  console.error("Failed to install jr:", err.message);
  process.exit(1);
});
