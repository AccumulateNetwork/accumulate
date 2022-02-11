resource "aws_ecs_cluster" "test_cluster" {
  name = "accumulate-test-cluster" # Name of cluster
}




data "aws_ecs_task_definition" "tool_service" {
  task_definition = "${aws_ecs_task_definition.tool_service[count.index].family}"
  count           = 4
}

resource "aws_ecs_task_definition" "tool_service" {
  count                    = 4
  depends_on               = [aws_ecs_cluster.test_cluster]
  family                   = "tool_service" # Name of first task
  container_definitions    = <<DEFINITION
  [
    {
      "name": "tool_service",
      "image": "registry.gitlab.com/accumulatenetwork/accumulate/accumulated:stable",
      "essential": true,
      "portMappings": [{"containerPort": 80}],
      "memory": 512,
      "cpu": 256,
      "logConfiguration": {
         "logDriver": "awslogs",
         "options": {
            "awslogs-group": "accumulate-testnet-log",
            "awslogs-region": "us-east-1",
            "awslogs-stream-prefix": "ecs"
                }
            },
      "command": ["init", "devnet", "--work-dir", "/mnt/efs/node", "--docker", "--bvns=3", "--followers=0", "--validators=4", "--dns-suffix", ".accumulate-testnet"],     
      "mountPoints": [
          {
              "containerPath": "/mnt/efs/node",
              "sourceVolume": "efs_temp",
              "readOnly": false
          }
      ]                 
    }
  ]
DEFINITION
  requires_compatibilities    = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode                = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                      = 512    # Specifying the memory our container requires
  cpu                         = 256     # Specifying the CPU our container requires
  volume {
    name     = "efs_temp"
    efs_volume_configuration {
      file_system_id = "${aws_efs_file_system.testnet.id}"
      root_directory = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id       = aws_efs_access_point.testnet.id
        iam                  = "ENABLED"

      }
    }
  }
  execution_role_arn          = "${aws_iam_role.ecsTaskExecutionRole.arn}"
  task_role_arn               = "${aws_iam_role.ecsTaskExecutionRole.arn}"
}
 



