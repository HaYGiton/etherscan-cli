"use strict";

const crypto = require("node:crypto");
const fs = require("node:fs");
const os = require("node:os");
const path = require("node:path");
const { spawnSync } = require("node:child_process");

const repositoryRoot = path.resolve(__dirname, "..");
const numericIdentifier = "(?:0|[1-9]\\d*)";
const releasePattern = new RegExp(
  `^v(${numericIdentifier}\\.${numericIdentifier}\\.${numericIdentifier}(?:-[0-9A-Za-z.-]+)?(?:\\+[0-9A-Za-z.-]+)?)$`,
);

const platforms = Object.freeze([
  { packageDir: "cli-darwin-arm64", os: "darwin", arch: "arm64", binary: "etherscan", extension: "tar.gz" },
  { packageDir: "cli-darwin-x64", os: "darwin", arch: "amd64", binary: "etherscan", extension: "tar.gz" },
  { packageDir: "cli-linux-arm64", os: "linux", arch: "arm64", binary: "etherscan", extension: "tar.gz" },
  { packageDir: "cli-linux-x64", os: "linux", arch: "amd64", binary: "etherscan", extension: "tar.gz" },
  { packageDir: "cli-win32-arm64", os: "windows", arch: "arm64", binary: "etherscan.exe", extension: "zip" },
  { packageDir: "cli-win32-x64", os: "windows", arch: "amd64", binary: "etherscan.exe", extension: "zip" },
]);

function parseReleaseTag(tag) {
  const match = releasePattern.exec(tag || "");
  if (!match) {
    throw new Error(`invalid release tag for npm: ${tag || "<empty>"}`);
  }
  return {
    version: match[1],
    distTag: match[1].includes("-") ? "next" : "latest",
  };
}

function run(command, args, options = {}) {
  let executable = command;
  let commandArgs = args;
  if (command === "npm" && process.env.npm_execpath) {
    executable = process.execPath;
    commandArgs = [process.env.npm_execpath, ...args];
  } else if (process.platform === "win32" && command === "npm") {
    executable = "npm.cmd";
  }
  const result = spawnSync(executable, commandArgs, {
    cwd: options.cwd || repositoryRoot,
    encoding: "utf8",
    stdio: options.inherit ? "inherit" : "pipe",
  });
  if (result.error) {
    throw result.error;
  }
  if (result.status !== 0) {
    const details = [result.stdout, result.stderr].filter(Boolean).join("\n").trim();
    throw new Error(`${executable} ${commandArgs.join(" ")} failed${details ? `:\n${details}` : ""}`);
  }
  return result.stdout || "";
}

function sha256(filePath) {
  return crypto.createHash("sha256").update(fs.readFileSync(filePath)).digest("hex");
}

function readChecksums(checksumPath) {
  const checksums = new Map();
  for (const line of fs.readFileSync(checksumPath, "utf8").split(/\r?\n/)) {
    const match = /^([0-9a-fA-F]{64})\s+\*?(.+)$/.exec(line.trim());
    if (match) {
      checksums.set(match[2], match[1].toLowerCase());
    }
  }
  return checksums;
}

function writeManifest(sourcePath, destinationPath, version, dependencyVersion = "") {
  const manifest = JSON.parse(fs.readFileSync(sourcePath, "utf8"));
  manifest.version = version;
  if (dependencyVersion && manifest.optionalDependencies) {
    for (const name of Object.keys(manifest.optionalDependencies)) {
      manifest.optionalDependencies[name] = dependencyVersion;
    }
  }
  fs.mkdirSync(path.dirname(destinationPath), { recursive: true });
  fs.writeFileSync(destinationPath, `${JSON.stringify(manifest, null, 2)}\n`);
  return manifest;
}

function extractBinary(archivePath, platform, destination) {
  const extractDir = fs.mkdtempSync(path.join(os.tmpdir(), "etherscan-npm-extract-"));
  try {
    let entries;
    if (platform.extension === "zip") {
      const listCommand = process.platform === "win32" ? ["tar", ["-tf", archivePath]] : ["unzip", ["-Z1", archivePath]];
      entries = run(listCommand[0], listCommand[1]).split(/\r?\n/).filter(Boolean);
      if (entries.filter((entry) => entry === platform.binary).length !== 1) {
        throw new Error(`${path.basename(archivePath)} must contain exactly one root-level ${platform.binary}`);
      }
      if (process.platform === "win32") {
        run("tar", ["-xf", archivePath, "-C", extractDir, platform.binary]);
      } else {
        run("unzip", ["-q", archivePath, platform.binary, "-d", extractDir]);
      }
    } else {
      entries = run("tar", ["-tzf", archivePath]).split(/\r?\n/).filter(Boolean);
      if (entries.filter((entry) => entry === platform.binary).length !== 1) {
        throw new Error(`${path.basename(archivePath)} must contain exactly one root-level ${platform.binary}`);
      }
      run("tar", ["-xzf", archivePath, "-C", extractDir, platform.binary]);
    }
    fs.copyFileSync(path.join(extractDir, platform.binary), destination);
    if (platform.extension !== "zip") {
      fs.chmodSync(destination, 0o755);
      // Windows filesystems do not expose Unix execute bits: chmod is a no-op
      // for them and stat continues to report 0666. The packed-tarball check
      // below is the cross-platform authority; this staging check is useful
      // only when the publishing host can represent Unix permissions.
      if (process.platform !== "win32") {
        const mode = fs.statSync(destination).mode & 0o777;
        if ((mode & 0o111) === 0) {
          throw new Error(`${platform.packageDir}/${platform.binary} is not executable after staging`);
        }
      }
    }
  } finally {
    fs.rmSync(extractDir, { recursive: true, force: true });
  }
}

function preparePackages(version, distDir, stageRoot) {
  const checksumPath = path.join(distDir, "checksums.txt");
  if (!fs.existsSync(checksumPath)) {
    throw new Error(`release checksums not found: ${checksumPath}`);
  }
  const checksums = readChecksums(checksumPath);
  const prepared = [];

  for (const platform of platforms) {
    const archiveName = `etherscan_${version}_${platform.os}_${platform.arch}.${platform.extension}`;
    const archivePath = path.join(distDir, archiveName);
    if (!fs.existsSync(archivePath)) {
      throw new Error(`release archive not found: ${archivePath}`);
    }
    const expected = checksums.get(archiveName);
    if (!expected) {
      throw new Error(`no checksum was published for ${archiveName}`);
    }
    const actual = sha256(archivePath);
    if (actual !== expected) {
      throw new Error(`checksum verification failed for ${archiveName}`);
    }

    const sourceDir = path.join(repositoryRoot, "npm", platform.packageDir);
    const packageDir = path.join(stageRoot, platform.packageDir);
    fs.mkdirSync(packageDir, { recursive: true });
    const manifest = writeManifest(
      path.join(sourceDir, "package.json"),
      path.join(packageDir, "package.json"),
      version,
    );
    fs.copyFileSync(path.join(repositoryRoot, "LICENSE"), path.join(packageDir, "LICENSE"));
    extractBinary(archivePath, platform, path.join(packageDir, platform.binary));
    prepared.push({
      name: manifest.name,
      directory: packageDir,
      binary: platform.binary,
      requiresExecutable: platform.extension !== "zip",
    });
  }

  const umbrellaDir = path.join(stageRoot, "cli");
  const umbrellaManifest = writeManifest(
    path.join(repositoryRoot, "package.json"),
    path.join(umbrellaDir, "package.json"),
    version,
    version,
  );
  for (const relative of [
    "LICENSE",
    "README.md",
    "npm/bin/etherscan.js",
    "npm/platform.js",
    "npm/prepublish-check.js",
  ]) {
    const source = path.join(repositoryRoot, relative);
    const destination = path.join(umbrellaDir, relative);
    fs.mkdirSync(path.dirname(destination), { recursive: true });
    fs.copyFileSync(source, destination);
  }
  prepared.push({ name: umbrellaManifest.name, directory: umbrellaDir });
  return prepared;
}

function packPackages(packages, packRoot) {
  fs.mkdirSync(packRoot, { recursive: true });

  return packages.map((pkg) => {
    // Publishing a directory runs prepublishOnly, but publishing the exact
    // tarball verified below does not. Preserve that release gate explicitly.
    if (pkg.name === "@etherscan/cli") {
      run(process.execPath, [path.join(pkg.directory, "npm", "prepublish-check.js")], {
        cwd: pkg.directory,
      });
    }

    const output = run("npm", [
      "pack",
      "--json",
      "--pack-destination",
      packRoot,
      pkg.directory,
    ]);
    let results;
    try {
      results = JSON.parse(output);
    } catch (error) {
      throw new Error(`npm pack returned invalid JSON for ${pkg.name}: ${error.message}`);
    }
    if (!Array.isArray(results) || results.length !== 1) {
      throw new Error(`npm pack must produce exactly one tarball for ${pkg.name}`);
    }

    const result = results[0];
    if (!result || typeof result.filename !== "string" || !Array.isArray(result.files)) {
      throw new Error(`npm pack returned incomplete metadata for ${pkg.name}`);
    }
    const tarball = path.resolve(packRoot, result.filename);
    if (path.dirname(tarball) !== path.resolve(packRoot) || !fs.existsSync(tarball)) {
      throw new Error(`npm pack did not create the expected tarball for ${pkg.name}`);
    }

    if (pkg.requiresExecutable) {
      const binary = result.files.find((entry) => entry.path === pkg.binary);
      if (!binary || !Number.isInteger(binary.mode)) {
        throw new Error(`${pkg.name} tarball does not contain mode metadata for ${pkg.binary}`);
      }
      if ((binary.mode & 0o111) === 0) {
        const mode = (binary.mode & 0o777).toString(8).padStart(3, "0");
        throw new Error(`${pkg.name} tarball contains non-executable ${pkg.binary} (mode ${mode})`);
      }
    }

    return { ...pkg, tarball };
  });
}

function isPublished(name, version) {
  const executable = process.platform === "win32" ? "npm.cmd" : "npm";
  const result = spawnSync(executable, ["view", `${name}@${version}`, "version", "--json"], {
    encoding: "utf8",
  });
  if (result.status === 0) {
    return true;
  }
  const output = `${result.stdout || ""}\n${result.stderr || ""}`;
  if (/E404|404 Not Found/.test(output)) {
    return false;
  }
  throw new Error(`could not query ${name}@${version}: ${output.trim()}`);
}

function publishPackages(packages, release) {
  for (const pkg of packages) {
    if (isPublished(pkg.name, release.version)) {
      console.log(`${pkg.name}@${release.version} is already published; skipping.`);
      continue;
    }
    const args = ["publish", `./${path.basename(pkg.tarball)}`, "--access", "public"];
    if (release.distTag !== "latest") {
      args.push("--tag", release.distTag);
    }
    if (process.env.NPM_PROVENANCE === "true") {
      args.push("--provenance");
    }
    run("npm", args, { cwd: path.dirname(pkg.tarball), inherit: true });
  }
}

function main() {
  const tag = process.env.GITHUB_REF_NAME || (process.env.VERSION ? `v${process.env.VERSION}` : "");
  const release = parseReleaseTag(tag);
  const distDir = path.resolve(process.env.DIST_DIR || path.join(repositoryRoot, "dist"));
  const stageRoot = fs.mkdtempSync(path.join(os.tmpdir(), "etherscan-npm-publish-"));
  try {
    const packages = preparePackages(release.version, distDir, stageRoot);
    const packed = packPackages(packages, path.join(stageRoot, "tarballs"));
    publishPackages(packed, release);
  } finally {
    fs.rmSync(stageRoot, { recursive: true, force: true });
  }
}

module.exports = {
  parseReleaseTag,
  platforms,
  readChecksums,
  preparePackages,
  packPackages,
};

if (require.main === module) {
  try {
    main();
  } catch (error) {
    console.error(`npm publish failed: ${error.message}`);
    process.exit(1);
  }
}
