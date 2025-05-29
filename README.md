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

[Zus](https://zus.network/) is the first S3-compatible storage platform that’s fast, secure, and ACID-compliant operating on a zero-trust network.
Our goal is to deliver 10x value to customers through:

- 5x better performance
- 2x lower costs, thanks to zero egress and API fees (for non-cloud deployments), and lower security and compute costs.
- 2x lower carbon footprint, enabled by our erasure-coded architecture
- Breachproof security with split-key, zero-knowledge, and erasure coded data
- 100% dynamic availability, with ability to add or replace servers on the fly
- Vendor neutrality, with no lock-in or dependency on a single storage provider
- One of our customers benchmarked our platform against AWS on [s3compare.io](https://s3compare.io) showing 5x performance gains.

### Core Features – Züs vs AWS S3 vs MinIO

| **Feature**                              | **AWS S3**                                          | **MinIO**                                      | **Züs**                                                                                   |
| ---------------------------------------- | --------------------------------------------------- | ---------------------------------------------- | ----------------------------------------------------------------------------------------- |
| **Managed Infrastructure**               | Fully managed with strong global uptime             | Self-hosted; requires manual setup and scaling | Fully managed infrastructure with flexible scaling                          |
| **Split-key Internal Breach Security**   | Not available; single-party access control          | Not available                                  | Built-in split-key security prevents internal breaches with decentralized key control      |
| **Zero Egress Fees**                     | Charges apply for all outbound data                 | No egress fees                                 | No egress fees on outbound traffic across providers (non-cloud option)                                       |
| **Zero API Fees**                        | Charges per API call                                | Free API access                                | Free unlimited API requests; ideal for high-frequency apps (non-cloud option)                               |
| **Encrypted Data Sharing**               | Requires external tools or complex configuration    | Not supported natively                         | Native proxy re-encryption enables secure, private sharing of encrypted files             |
| **Zero Trust Network**               | Not supported                                       | Not supported                                  | Zero-trust architecture ensures providers can't access file contents or user identity |
| **ACID Compliant (Data Integrity)**      | Not ACID compliant            | Not ACID compliant                    | Fully ACID compliant to ensure consistent reads/writes and verifiable storage behavior    |
| **Add/Swap Infrastructure (No Lock-in, 100% Dynamic Availability)** | No real-time server switching | Tied to fixed infrastructure                   | Add, remove, or swap storage providers dynamically with no lock-in for 100% dynamic availability                        |

## Configuration

**Prerequisites**

- Have a wallet and allocation on Züs. Allocation can be created through [Blimp](blimp.software) or [Vult](vult.network)
    - Unsure how to recover a wallet and allocation using the CLI, check steps 1-3 here: https://docs.zus.network/zus-docs/clis
- Have your wallet.json in ~/.zcn
```json
{"client_id":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
"client_key":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
"keys":[{"public_key":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
"private_key":"xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"}],
"mnemonics":"xxxx xxxx xxxx xxxxx",
"version":"1.0","date_created":"2023-05-03T12:44:46+05:30","nonce":0,"is_split":false}
```
- Have your config.yaml in ~/.zcn
```yaml
block_worker: https://mainnet.zus.network/dns
signature_scheme: bls0chain
min_submit: 50 # in percentage
min_confirmation: 50 # in percentage
confirmation_chain_length: 3
```

**Set Up**

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
name> myZus
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
Config Directory - directory to read config files (defaults to ~/.zcn; make sure to use the correct windows path for  `C:\Users\Username\.zcn`).
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

Make sure your rclone.conf file is created.
- For Windows, check %APPDATA%\rclone\rclone.conf. If you downloaded the rclone.exe, you can place the rclone.conf in the same directory as the .exe.
- For macOS/Linux, check ~/.config/rclone/rclone.conf

Example rclone.conf :
```ini
[myZus]
type = zus
allocation_id = <allocation_id>
```

Once configured you can then use `rclone` like this,

See top level directories

    rclone lsd myZus:

Output example:

```
  -1 2025-05-14 15:27:59        -1 Encrypted
```

Make a new directory (This example shows new directory name as "directory")

    rclone mkdir myZus:directory

List the contents of a directory

    rclone ls myZus:directory

Sync `/home/local/directory` to the remote path, deleting any
excess files in the path.

    rclone sync --interactive /home/local/directory myZus:directory

You can also check your allocation in the Blimp and Vult UI. Files should be in a folder named "directory".
