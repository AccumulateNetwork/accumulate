# Running a local DevNet

Given `${DIR}` is your chosen configuration directory:

1. Configure the devnet: `accumulated init devnet -w ${DIR} --reset`
2. Run the devnet: `accumulated run devnet -w ${DIR}`
3. Configure the CLI: `export ACC_API="http://127.0.1.1:26660/v1"`
4. Have fun!

# Running a DevNet with Docker Compose

Given `${DIR}` is your chosen configuration directory:

1. Configure docker compose: `accumulated init devnet -w ${DIR} --reset --compose-only`
2. Configure the devnet: `cd ${DIR} && docker-compose run scripts accumulated init devnet -w /node --docker`
3. Run the devnet: `docker-compose up`
4. Start a shell: `docker-compose run --rm script bash`
5. Have fun!

If the container OS and host OS are the same, replace step 1 and 2 with step 1
from the previous section.

To run the CLI on your host OS (instead of within a Docker container), set
`export ACC_API="http://localhost:26660/v1"`.