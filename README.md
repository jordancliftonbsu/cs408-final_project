### Deployment Prerequisites
1. Download your EC2 key pair (`.pem` file) from AWS.
2. Move it to your local SSH directory and restrict permissions:
   ```bash
   mv ~/Downloads/your-key.pem ~/.ssh/
   chmod 400 ~/.ssh/your-key.pem