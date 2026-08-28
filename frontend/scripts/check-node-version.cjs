const major = Number(process.versions.node.split('.')[0]);

if (!Number.isInteger(major) || major < 20) {
  console.error(
    `Cortex frontend requires Node.js >=20; current version is ${process.versions.node}.`,
  );
  process.exit(1);
}
