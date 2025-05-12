[<img src="https://rclone.org/img/logo_on_light__horizontal_color.svg" width="50%" alt="rclone logo">](https://rclone.org/#gh-light-mode-only)
[<img src="https://rclone.org/img/logo_on_dark__horizontal_color.svg" width="50%" alt="rclone logo">](https://rclone.org/#gh-dark-mode-only)

[Website](https://rclone.org) |
[Documentation](https://rclone.org/docs/) |
[Download](https://rclone.org/downloads/) |
[Contributing](CONTRIBUTING.md) |
[Changelog](https://rclone.org/changelog/) |
[Installation](https://rclone.org/install/) |
[Forum](https://forum.rclone.org/)

## Züs Overview

[Zus](https://zus.network/) is the first S3-compatible storage platform that’s both ACID-compliant and operates on a zero-knowledge network — meaning you no longer need additional services like AWS Athena or GuardDuty to secure or query your data.
Our goal is to deliver 10x value to customers through:

- 5x better performance
- 2x lower costs, thanks to zero egress and API fees
- 2x lower carbon footprint, enabled by our erasure-coded architecture
  Bulletproof security with split-key, zero-knowledge, and erasure coded data
  Vendor neutrality, with no lock-in or dependency on a single storage provider
  One of our customers recently benchmarked our platform against AWS on [s3compare.io](https://s3compare.io) and found 5x performance gains across real-world scenarios.
  We also fill security and vendor neutrality gaps that MinIO and AWS have in their solution
  Beyond backup and datalake storage, our platform is ideal for storing AI data, where integrity and verifiability matter such as for MCP workflows.

### Core Features – Züs vs AWS S3 vs MinIO

| **Feature**                              | **AWS S3**                                          | **MinIO**                                      | **Züs**                                                                                   |
| ---------------------------------------- | --------------------------------------------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------------- |
| **Managed Infrastructure**               | Fully managed with strong global uptime             | Self-hosted; requires manual setup and scaling | Fully managed decentralized infrastructure with flexible scaling                          |
| **Split-key Internal Breach Security**   | Not available; single-party access control          | Not available                                  | Built-in split-key security prevents internal breaches by decentralizing key control      |
| **Zero Egress Fees**                     | Charges apply for all outbound data                 | No egress fees                                 | No egress fees on outbound traffic across providers                                       |
| **Zero API Fees**                        | Charges per API call                                | Free API access                                | Free unlimited API requests; ideal for high-frequency apps                                |
| **Encrypted Data Sharing**               | Requires external tools or complex configuration    | Not supported natively                         | Native proxy re-encryption enables secure, private sharing of encrypted files             |
| **Zero Knowledge Network**               | Not supported                                       | Not supported                                  | Zero-knowledge architecture ensures providers can't access file contents or user identity |
| **ACID Compliant (Data Integrity)**      | Eventual consistency; not ACID compliant            | No built-in ACID guarantees                    | Fully ACID compliant to ensure consistent reads/writes and verifiable storage behavior    |
| **Add/Swap Infrastructure (No Lock-in)** | Vendor lock-in with no real-time provider switching | Tied to fixed infrastructure                   | Add, remove, or swap storage providers dynamically with no lock-in                        |

## Configuration

Here is an example of how to make a `zus` remote called `myZus`.

First run

    rclone config

This will guide you through an interactive setup process:

```
No remotes found, make a new one?
n) New remote
s) Set configuration password
q) Quit config
n/s/q> n
name> mySia
Type of storage to configure.
Enter a string value. Press Enter for the default ("").
Choose a number from below, or type in your own value
...
59 / Zus Decentralized Storage
   \ "zus"
...
Storage> zus
Zus Allocation ID - allocation ID.
allocation_id>
Config Directory - directory to read config files(defaults to ~/.zcn).
config_dir>
Work Directory - directory to read/write files.
work_dir>
Encrypt - encrypt the data before uploading.
y) Yes
n) No (default)
y/n> n
Edit advanced config?
y) Yes
n) No (default)
y/n> n
Configuration complete.
Options:
- type: zus
- allocation_id: allocation_id
Keep this "myZus" remote?
y) Yes this is OK (default)
e) Edit this remote
d) Delete this remote
y/e/d> y
```

Once configured you can then use `rclone` like this,

See top level directories

    rclone lsd remote:

Make a new directory

    rclone mkdir remote:directory

List the contents of a directory

    rclone ls remote:directory

Sync `/home/local/directory` to the remote path, deleting any
excess files in the path.

    rclone sync --interactive /home/local/directory remote:directory
