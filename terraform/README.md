1. Terraform Command Steps;
    *   terraform init - to initialize the backend.
    *   terraform plan - to create an execution plan. You will be prompted to provide your gitlab token.
    *   terraform apply - to apply resources to aws. You will be prompted to provide your gitlab token.

2. Steps in AWS console;
    *   Go to Elastic Container Services.
    *   Click on task definitions on the left menu bar.
    *   Select dev_tools (for Devnet) or tool_service (for Testnet)
    *   Click on Action at the top menu bar and select "Run Task" on the dropdown.
        *   Launch Type - Fargate.
        *   Cluster - accumulate-dev-cluster (for Devnet) or accumulate-test-cluster (for Testnet).
        *   Number of tasks - 4 
        *   Platform Version - 1.4.0
        *   Cluster VPC - accumulate-devnet (for Devnet) or test_net (for Testnet).
        *   Subnets - Choose all 4 subnets.
        *   Security Groups - Click edit then select "choose existing security group". Select accumulate-devnet-tools (for Devnet) or accumulate-testnet-tools (for Testnet).
        *   Auto-assign public IP should be enebled.
        *   Run Task.
    *   Once the 4 tasks run and are successful, click on Stop All in order to restart the nodes to read the config files in EFS.

The above "AWS Console Steps" is being done manually due to the fact that the tool Service of this application needs to stay stopped after it exits with exit code 0 and not restart again. I am working on a way to automate it in the terraform configuration file.


