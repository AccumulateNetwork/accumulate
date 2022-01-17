resource "aws_iam_role" "ecsTaskExecutionRole_1" {
  name               = "accumulate-devnet-ecs"
  assume_role_policy = "${data.aws_iam_policy_document.assume_role_policy_1.json}"
}

data "aws_iam_policy_document" "assume_role_policy_1" {
  statement {
      actions = ["sts:AssumeRole"]

    principals {
        type        = "Service"
        identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}


resource "aws_iam_role_policy_attachment" "ecsTaskExecutionRole_policy_1" {
  role       = "${aws_iam_role.ecsTaskExecutionRole_1.name}"
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}