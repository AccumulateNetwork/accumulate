locals {
  base_container = {
    image: "public.ecr.aws/accumulate-network/accumulate/accumulated:develop",
    essential: true
    portMappings: [
      { containerPort: 8080  }, // Website
      { containerPort: 26656 }, // Tendermint P2P
      { containerPort: 26657 }, // Tendermint JSON-RPC
      { containerPort: 26658 }, // Tendermint gRPC
      { containerPort: 26660 }, // Accumulate JSON-RPC
      { containerPort: 26662 }, // Prometheus
    ]
    memory: 512
    cpu: 256
    logConfiguration: {
      logDriver: "awslogs"
      options: {
        awslogs-group: aws_cloudwatch_log_group.devnet_log.name
        awslogs-region: var.aws_region
        awslogs-stream-prefix: "ecs"
      }
    }
    mountPoints: [
      {
        containerPath: "/mnt/efs/node"
        sourceVolume: "accumulate-devnet-storage"
        readOnly: false
      }
    ]
  }

  init_once = merge({
    name: "init-once"
    command: [
      "init", "devnet",
      "--docker",
      "--work-dir=/mnt/efs/node",
      "--bvns=3",
      "--followers=0",
      "--validators=4",
      "--dns-suffix=.accumulate-devnet",
      "--log-levels=error;accumulate=debug;executor=debug;governor=debug",
    ]
  }, local.base_container)

  init_reset = merge({
    name: "init-reset"
    command: [
      "init", "devnet", "--reset",
      "--docker",
      "--work-dir=/mnt/efs/node",
      "--bvns=3",
      "--followers=0",
      "--validators=4",
      "--dns-suffix=.accumulate-devnet",
      "--log-levels=error;accumulate=debug;executor=debug;governor=debug",
    ]
  }, local.base_container)

  nodes = flatten([
    for subnet in concat(["dn"], [for n in range(0, 3) : "bvn${n}" ]) : [
      for n in range(0, 4) : merge({
        name: "${subnet}-${n}"
        command: ["run", "-w", "/mnt/efs/node/${subnet}/Node${n}"]
      }, local.base_container)
    ]
  ])

  containers = concat(local.nodes, [local.init_once, local.init_reset])
}

resource "aws_ecs_cluster" "dev_cluster" {
  name = "accumulate-dev-cluster"
}

resource "aws_security_group" "nodes" {
  vpc_id = aws_vpc.dev_vpc.id
  name   = "accumulate-devnet-nodes"

  # Allow node website
  ingress {
    from_port        = 8080
    to_port          = 8080
    protocol         = "tcp"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }

  # Allow node services
  ingress {
    from_port        = 26656
    to_port          = 26666
    protocol         = "tcp"
    cidr_blocks      = ["0.0.0.0/0"]
    ipv6_cidr_blocks = ["::/0"]
  }
}

resource "aws_service_discovery_service" "accumulate" {
  for_each = { for node in local.nodes : node.name => node }

  name = each.key

  dns_config {
    routing_policy = "MULTIVALUE"
    namespace_id   = aws_service_discovery_private_dns_namespace.devnet.id

    dns_records {
      ttl  = 10
      type = "A"
    }
  }

  health_check_custom_config {
    failure_threshold = 1
  }
}

resource "aws_ecs_task_definition" "accumulate" {
  for_each = { for c in local.containers : c.name => c }

  lifecycle {
    ignore_changes = [volume]
  }

  family                   = "accumulate-devnet-${each.key}"
  container_definitions    = jsonencode([each.value])
  requires_compatibilities = ["FARGATE"] # Stating that we are using ECS Fargate
  network_mode             = "awsvpc"    # Using awsvpc as our network mode as this is a  Fargate requirement
  memory                   = 512         # Specifying the memory our container requires
  cpu                      = 256         # Specifying the CPU our container requires
  execution_role_arn       = aws_iam_role.ecsTaskExecutionRole_1.arn
  task_role_arn            = aws_iam_role.ecsTaskExecutionRole_1.arn

  volume {
    name = "accumulate-devnet-storage"
    efs_volume_configuration {
      file_system_id          = aws_efs_file_system.devnet.id
      root_directory          = "/mnt/efs/node"
      transit_encryption      = "ENABLED"
      transit_encryption_port = 2999
      authorization_config {
        access_point_id = aws_efs_access_point.devnet.id
        iam = "ENABLED"
      }
    }
  }
}

resource "aws_ecs_service" "accumulate" {
  for_each = { for node in local.nodes : node.name => node }

  name             = "accumulate-devnet-${each.key}"
  cluster          = aws_ecs_cluster.dev_cluster.id
  task_definition  = "${aws_ecs_task_definition.accumulate[each.key].arn}"
  launch_type      = "FARGATE"
  desired_count    = var.run ? 1 : 0
  platform_version = "1.4.0"
  depends_on       = [aws_alb_listener.dev_listener]

  network_configuration {
    assign_public_ip = true # Provide our containers with public IPs
    subnets          = [aws_subnet.dev_pubsub_a.id,aws_subnet.dev_pubsub_b.id]
    security_groups  = [aws_security_group.nodes.id]
  }

  load_balancer {
    target_group_arn = aws_alb_target_group.dev_target.arn
    container_name   = each.key
    container_port   = 26660
  }

  service_registries {
    registry_arn   = aws_service_discovery_service.accumulate[each.key].arn
    container_name = each.key
  }
}