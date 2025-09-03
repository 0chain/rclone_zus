
[<img src="https://rclone.org/img/logo_on_light__horizontal_color.svg" width="50%" alt="rclone logo">](https://rclone.org/#gh-light-mode-only)
[<img src="https://rclone.org/img/logo_on_dark__horizontal_color.svg" width="50%" alt="rclone logo">](https://rclone.org/#gh-dark-mode-only)

[Rclone for Züs](#what-is-rclone_zus
) |
[Installation](#installation--setup) |
[Website](https://rclone.org) |
[Documentation](https://rclone.org/docs/) |
[Download](https://rclone.org/downloads/) |
[Contributing](CONTRIBUTING.md) |
[Changelog](https://rclone.org/changelog/) |
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

## What is rclone_zus?

**rclone_zus** is a custom integration of the rclone command-line tool with the Züs decentralized cloud. It enables users to interact with Züs storage using familiar rclone commands like `copy, sync, move, and ls`.

This backend implementation allows developers, DevOps teams, and cloud users to:

- Use rclone’s powerful CLI and scripting capabilities with Züs

- Perform efficient, **server-side batch operations** (copy, delete, move)

- Use Züs as an S3-compatible remote via rclone without vendor lock-in

 The zus backend is now available via -type zus in your rclone.conf file and supports advanced Züs features including:

- `Sync & batch mode` uploads

- Server-side copy & move

- Native delete, purge, and stat support

- Directory listings and recursive operations

###  Why Use rclone_zus?

- Scripting Ready: Automate uploads/downloads via shell scripts

- Dev & CI Friendly: Plug into CI/CD pipelines with secure Züs backend

- Zero Lock-in: Maintain open architecture with CLI-driven usage

- Use [Vult](https://vult.network) or [Blimp UI](https://blimp.network) to manage and render files in a carousel view

- Share both public and encrypted files or folders with anyone instantly

- Seamlessly scale by managing multiple allocations via Blimp and organizing data into multiple data rooms

- Fast Sync: Avoid redundant uploads with batch commit

### Allocation Performance

**For reliable performance:**
- Use Züs blobbers usually provide better stability and performance

## Configuration

**Prerequisites**

Before using `rclone_zus`, you must have a wallet, allocation, and configuration files in place.

### 1. Download Wallet via Blimp or Vult (Default Method)

The standard way to configure your Züs wallet is by downloading it through the **Blimp** or **Vult** user interfaces. This requires no command-line setup and ensures all required files are prepared for you.

#### Downloading from Blimp

1. Visit [**Blimp**](https://blimp.zus.network)
2. Navigate to **Manage Allocation**
3. Select your allocation
4. Click the **ellipsis (⋯)** button
5. Choose **“Download Wallet”**
6. Enter your **mnemonic** or **wallet password**
7. You’ll receive a `.zip` file containing:
   - `wallet.json` – Your Züs wallet credentials
   - `allocation.txt` – The Allocation ID
   - `config.yaml` – Züs network configuration (block worker, signature scheme, etc.)
  
<img width="1000" height="1000" alt="image" src="https://github.com/user-attachments/assets/205acb0f-9c5e-4c88-94a8-61cb41fab10c" />


#### Move Files to Config Directory

Extract the ZIP and move **all three files** to your system’s default config folder:

- **Windows**:  
  `C:\Users\<your-username>\.zcn`

- **Linux/macOS**:  
  `~/.zcn/`

> If the `.zcn` folder does not exist, create it manually.

---

#### Downloading from Vult

1. Visit [**Vult**](https://vult.zus.network)
2. Click your **username** (top right), then go to **Profile & Wallet**
3. Scroll to the bottom and click **“Download Wallet”**
4. A `.zip` file will be downloaded with the same three files:
   - `wallet.json`
   - `allocation.txt`
   - `config.yaml`
5. Move them into your `.zcn` folder (`~/.zcn` or `C:\Users\...\ .zcn`) as described above.

<img width="2305" height="1902" alt="image" src="https://github.com/user-attachments/assets/10af6f19-cb41-4de1-bf8e-376b5ff964cd" />

### Switching Between Blimp and Vult
If you switch between Blimp and Vult allocations, ensure that:

- You replace `wallet.json` with the version linked to the correct wallet
- You replace `allocation.txt` with the matching allocation ID
- You can reuse `config.yaml` as long as it points to the same Züs network (e.g. mainnet)

> The `rclone_zus` CLI reads `wallet.json` and `allocation.txt` from the `.zcn` folder each time a command runs.
>

### 2. Alternate Setup (CLI Method)

Alternatively, you can create your wallet and allocation using the CLI.

#### Steps:

1. Create a wallet and allocation via [Züs CLI tools](https://docs.zus.network/zus-docs/clis)
2. Place the following files in your `~/.zcn` folder (or Windows equivalent):

##### `wallet.json`

```json
{
  "client_id": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "client_key": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
  "keys": [
    {
      "public_key": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx",
      "private_key": "xxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxxx"
    }
  ],
  "mnemonics": "xxxx xxxx xxxx xxxx xxxx xxxx",
  "version": "1.0",
  "date_created": "2023-05-03T12:44:46+05:30",
  "nonce": 0,
  "is_split": false
}
```

##### `config.yaml`

```yaml
block_worker: https://mainnet.zus.network/dns
signature_scheme: bls0chain
min_submit: 50
min_confirmation: 50
confirmation_chain_length: 3
```

##### `allocation.txt`

```
<your allocation ID>
```

> ⚠Ensure the CLI-generated wallet matches the allocation you're trying to access.
> This method is more error-prone for beginners and should only be used if you're familiar with the Züs CLI ecosystem.

## Installation & Setup

This section guides you through cloning, building, and configuring rclone_zus with the Züs backend.
### 1. Clone the Repository

    git clone https://github.com/0chain/rclone_zus.git
    cd rclone_zus

### 2. Build the Rclone Binary

Use the provided Makefile to build the project:

    make

This will compile the rclone binary into the project root (./rclone), including the Züs backend.

💡 Troubleshooting: If make fails (e.g., missing make command or incompatible system), you can build manually:

    go build -o rclone ./rclone.go

Ensure you have Go ≥1.20 installed (suggested go 1.23.4)and your GOPATH properly configured.

This will build a local ./rclone binary with the Züs backend integrated.

Note: If you're modifying backend code (e.g. backend/zus/zus.go), you can recompile by running the go build command again.

### 3. (Optional) Install as Global Command rclone_zus

To use your custom Rclone binary without the ./ prefix, install it globally by copying it to a directory in your system's $PATH, such as /usr/local/bin:

    sudo cp ./rclone /usr/local/bin/rclone_zus

After this, you can run it from anywhere as a normal command:

    rclone_zus move TestZus:/source TestZus:/dest

📌 Why rename it?

Renaming it to rclone_zus helps avoid conflicts with the system-installed rclone, if present.

### 4. Configure Züs SDK

Ensure the following Züs config files are present in `~/.zcn/`:

- `wallet.json` – Züs wallet
- `config.yaml` – Züs network configuration
- `allocation.txt` – Your allocation ID (64-character hex string)

You can generate these using:

- [Züs CLI](https://docs.zus.network/zus-docs/clis)
- [Vult UI](https://vult.network)
- [Blimp UI](https://blimp.software)

**Remote Cofiguration**

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
Make sure your **rclone.conf** file is created.
**Finding rclone.conf:**
- To locate your rclone configuration file (`rclone.conf`) via command line, use the command `rclone config file`
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

    rclone lsd <remote name>:<absolute path>

Example to list all the files and directories inside the root directory of the remote "myZus"

    rclone lsd myZus:/

Output example:

```
    -1 2025-05-14 15:27:59        -1 Encrypted
    -1 2025-07-12 17:25:15        -1 10MbFiles100
    -1 2025-07-12 17:44:35        -1 10MbFiles100M
    -1 2025-07-12 17:46:51        -1 project-zus
    -1 2025-07-14 22:45:57        -1 10MbFiles50
```

**Make a new directory** 

    rclone mkdir myZus:<path>/<new_directory_name>

Example: create new direcotry in the root (This example shows new directory name as "newDirectory")

    rclone mkdir myZus:/newDirectory

**List** the contents of a directory

    rclone ls myZus:/<directory_path>

**Copy** from source to destination `(Local to Remote, Remote to Remote, Remote to Local)`

    `rclone copy <source_remote>:<source_path> <target_remote>:<target_path>` 
    
- **Note**: Copy/move/sync commands only work within the same remote (same allocation). You cannot copy/move/sync across two different remotes (different allocations). 

**Local to Züs Examples:**
```bash
# Windows example - copying from local Windows path to Züs remote
rclone copy "C:\Users\<username>\OneDrive\Desktop\New folder" myZus:/testDirectory

# Linux/macOS example - copying from local Unix path to Züs remote  
rclone copy /home/user/documents myZus:/backup
```

**Züs to Local Examples:**
```bash
# Copying from Züs remote to local directory
rclone copy myZus:/documents /home/user/downloads
```

**Cross-Cloud Backup Examples (Google Drive ↔ Züs):**
```bash
# Google Drive to Züs backup (source: gdrive, target: myZus)
rclone copy gdrive:important-files myZus:/backup

# Züs to Google Drive backup (source: myZus, target: gdrive)
rclone copy myZus:/documents gdrive:zus-backup
```

**Same Remote Operations (within same allocation):**
```bash
# Copying within the same Züs remote (source: myZus, target: myZus)
rclone copy myZus:/sourcefilesDir/ myZus:/destinationDir/
```


**Move** from source to destination `(Local to Remote, Remote to Remote, Remote to Local)`

    `rclone move <source_remote>:<source_path> <target_remote>:<target_path>` 
    
- **Cross-Remote Limitation**: Same limitation as copy - only works within the same remote/allocation

**Local to Züs Examples:**
```bash
# Windows example - moving from local Windows path to Züs remote
rclone move "C:\Users\<username>\Desktop\New folder" myZus:/testDirectory

# Linux/macOS example - moving from local Unix path to Züs remote
rclone move /home/user/documents myZus:/backup
```

**Züs to Local Examples:**
```bash
# Moving from Züs remote to local directory
rclone move myZus:/documents /home/user/downloads
```

**Cross-Cloud Examples (Google Drive ↔ Züs):**
```bash
# Google Drive to Züs (source remote: gdrive, target remote: myZus)
rclone move gdrive:important-files myZus:/backup

# Züs to Google Drive (source remote: myZus, target remote: gdrive)
rclone move myZus:/documents gdrive:zus-backup
```

**Same Remote Operations (within same allocation):**
```bash
# Moving within the same Züs remote (source remote: myZus, target remote: myZus)
rclone move myZus:/sourcefilesDir/ myZus:/destinationDir/
```

Sync `/home/local/directory` to the remote path, deleting any
excess files in the path.

    rclone sync --interactive /home/local/directory myZus:directory

You can also check your allocation in the Blimp and Vult UI. Files should be in a folder named "directory".


## Sync Mode Configuration
### Use sync mode in rclone_zus for bulk operations

use `--transfers=number of operations | --transfers=50` with the commands.

```
Edit advanced config?
y) Yes
n) No (default)
y/n> y

Option sdk_log_level.
Log level for the SDK
Enter a signed integer. Press Enter for the default (0).
sdk_log_level> leave empty

Option batch_mode.
Upload file batching sync|async|off.
This sets the batch mode used by rclone.
This has 3 possible values
- off - no batching
- sync - batch uploads and check completion (default)
- async - batch upload and don't check completion
Rclone will close any outstanding batches when it exits which may make
a delay on quit.
Enter a value of type string. Press Enter for the default (sync).
batch_mode> leave empty

Option batch_size.
Max number of files in upload batch.
This sets the batch size of files to upload. It has to be less than 50.
By default this is 0 which means rclone will calculate the batch size
depending on the setting of batch_mode.
- batch_mode: async - default batch_size is 100
- batch_mode: sync - default batch_size is the same as --transfers
- batch_mode: off - not in use
Rclone will close any outstanding batches when it exits which may make
a delay on quit.
Setting this is a great idea if you are uploading lots of small files
as it will make them a lot quicker. You can use --transfers 32 to
maximise throughput.
Enter a signed integer. Press Enter for the default (0).
batch_size> 50

Option batch_timeout.
Max time to allow an idle upload batch before uploading.
If an upload batch is idle for more than this long then it will be
uploaded.
The default for this is 0 which means rclone will choose a sensible
default based on the batch_mode in use.
- batch_mode: async - default batch_timeout is 5s
- batch_mode: sync - default batch_timeout is 500ms
- batch_mode: off - not in use
Enter a duration s,m,h,d,w,M,y. Press Enter for the default (0s).
batch_timeout> leave empty

Option batch_commit_timeout.
Max time to wait for a batch to finish committing
Enter a duration s,m,h,d,w,M,y. Press Enter for the default (10m0s).
batch_commit_timeout> leave empty

Option description.
Description of the remote.
Enter a value. Press Enter to leave empty.
description> leave empty

Edit advanced config?
y) Yes
n) No (default)
y/n> n

Configuration complete.
```


**Copy** from source to destination `(Local to Remote, Remote to Remote, Remote to Local)`

    rclone copy <remote name>:<source path> <remote name>:<destination path>  --transfers=50

**Move** from source to destination `(Local to Remote, Remote to Remote, Remote to Local)`

    rclone move <remote name>:<source path> <remote name>:<destination path>  --transfers=50
