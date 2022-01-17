resource "aws_iam_role" "ecsTaskExecutionRole" {
  name               = "ecsTaskExecutionRole"
  assume_role_policy = "${data.aws_iam_policy_document.assume_role_policy.json}"
}

data "aws_iam_policy_document" "assume_role_policy" {
  statement {
    actions       = ["sts:AssumeRole"]

    principals {
      type        = "Service"
      identifiers = ["ecs-tasks.amazonaws.com"]
    }
  }
}


resource "aws_iam_role_policy_attachment" "ecsTaskExecutionRole_policy" {
  role       = "${aws_iam_role.ecsTaskExecutionRole.name}"
  policy_arn = "arn:aws:iam::aws:policy/service-role/AmazonECSTaskExecutionRolePolicy"
}

# resource "aws_iam_role" "scheduled_task_cloudwatch" {
#   name               = "accumulate-cloudwatch-role"
#   assume_role_policy = <<EOF
# {
#   "Version": "2012-10-17",
#   "Statement": [
#     {
#       "Action": "sts:AssumeRole",
#       "Principal": {
#         "Service": "events.amazonaws.com"
#       },
#       "Effect": "Allow",
#       "Sid": ""
#     }
#   ]
# }
# EOF
# }

# resource "aws_iam_role_policy" "scheduled_task_cloudwatch_policy" {
#   name   = "accumulate-cloudwatch-policy"
#   role   = aws_iam_role.scheduled_task_cloudwatch.id
#   policy = <<EOF
# {
#   "Version": "2012-10-17",
#   "Statement": [
#     {
#       "Effect": "Allow",
#       "Action": [
#         "ecs:RunTask"
#       ],
#       "Resource": [
#         "${replace(aws_ecs_task_definition.tool_service.arn, "/:\\d+$/", ":*")}"
#       ]
#     },
#     {
#       "Effect": "Allow",
#       "Action": "iam:PassRole",
#       "Resource": [
#         "*"
#       ]
#     }
#   ]
# }
# EOF
# }